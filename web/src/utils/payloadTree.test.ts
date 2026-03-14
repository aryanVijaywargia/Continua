import { describe, expect, it } from 'vitest';
import {
  buildPayloadTree,
  collectExpandablePaths,
  collectInitiallyExpandedPaths,
  findPayloadMatches,
} from './payloadTree';

describe('payloadTree', () => {
  it('builds object and array nodes with stable metadata', () => {
    const tree = buildPayloadTree({
      model: 'gpt-4',
      messages: [{ role: 'user' }],
    });

    expect(tree.type).toBe('object');
    expect(tree.childCount).toBe(2);
    expect(tree.subtreeNodeCount).toBe(5);

    if (tree.type !== 'object') {
      throw new Error('Expected object root');
    }

    expect(tree.children[0]).toMatchObject({
      key: 'model',
      path: '$.model',
      type: 'primitive',
    });
    expect(tree.children[1]).toMatchObject({
      key: 'messages',
      path: '$.messages',
      type: 'array',
    });
  });

  it('uses collision-safe paths for object keys that look like array accessors', () => {
    const tree = buildPayloadTree({
      'a[0]': {
        nested: 'match first',
      },
      a: [
        {
          nested: 'match second',
        },
      ],
      '0': 'string key',
    });

    if (tree.type !== 'object') {
      throw new Error('Expected object root');
    }

    const bracketKeyNode = tree.children.find((child) => child.key === 'a[0]');
    const arrayNode = tree.children.find((child) => child.key === 'a');
    const numericStringNode = tree.children.find((child) => child.key === '0');

    expect(bracketKeyNode).toMatchObject({
      key: 'a[0]',
      path: '$["a[0]"]',
      type: 'object',
    });
    expect(arrayNode).toMatchObject({
      key: 'a',
      path: '$.a',
      type: 'array',
    });
    expect(numericStringNode).toMatchObject({
      key: '0',
      path: '$["0"]',
      type: 'primitive',
    });

    if (
      !bracketKeyNode ||
      !arrayNode ||
      !numericStringNode ||
      bracketKeyNode.type !== 'object' ||
      arrayNode.type !== 'array'
    ) {
      throw new Error('Expected collection children');
    }

    expect(bracketKeyNode.children[0]).toMatchObject({
      key: 'nested',
      path: '$["a[0]"].nested',
      type: 'primitive',
    });
    expect(arrayNode.children[0]).toMatchObject({
      key: 0,
      path: '$.a[0]',
      type: 'object',
    });

    if (arrayNode.children[0].type !== 'object') {
      throw new Error('Expected nested object node');
    }

    expect(arrayNode.children[0].children[0]).toMatchObject({
      key: 'nested',
      path: '$.a[0].nested',
      type: 'primitive',
    });

    const matchIds = findPayloadMatches(tree, 'match').matches.map((match) => match.id);
    expect(matchIds).toEqual([
      '$["a[0]"].nested:value',
      '$.a[0].nested:value',
    ]);
    expect(new Set(matchIds).size).toBe(matchIds.length);
  });

  it('builds primitive nodes for scalars and null values', () => {
    expect(buildPayloadTree('hello')).toMatchObject({
      type: 'primitive',
      value: 'hello',
      path: '$',
    });
    expect(buildPayloadTree(42)).toMatchObject({
      type: 'primitive',
      value: 42,
      path: '$',
    });
    expect(buildPayloadTree(false)).toMatchObject({
      type: 'primitive',
      value: false,
      path: '$',
    });
    expect(buildPayloadTree(null)).toMatchObject({
      type: 'primitive',
      value: null,
      path: '$',
    });
  });

  it('handles empty collections with expanded roots', () => {
    const emptyObject = buildPayloadTree({});
    const emptyArray = buildPayloadTree([]);

    expect(emptyObject).toMatchObject({
      type: 'object',
      childCount: 0,
      defaultExpanded: true,
    });
    expect(emptyArray).toMatchObject({
      type: 'array',
      childCount: 0,
      defaultExpanded: true,
    });
  });

  it('applies initial expansion rules for deep nesting and wide arrays', () => {
    const wideArray = Array.from({ length: 201 }, (_, index) => index);
    const tree = buildPayloadTree({
      nested: {
        deeper: {
          leaf: true,
        },
      },
      wide: wideArray,
    });

    if (tree.type !== 'object') {
      throw new Error('Expected object root');
    }

    const nestedNode = tree.children[0];
    const wideNode = tree.children[1];

    expect(tree.defaultExpanded).toBe(true);
    expect(nestedNode).toMatchObject({
      type: 'object',
      defaultExpanded: true,
      depth: 1,
    });
    expect(wideNode).toMatchObject({
      type: 'array',
      defaultExpanded: false,
      depth: 1,
      childCount: 201,
    });

    if (nestedNode.type !== 'object') {
      throw new Error('Expected nested object node');
    }

    const deeperNode = nestedNode.children[0];
    expect(deeperNode).toMatchObject({
      type: 'object',
      defaultExpanded: false,
      depth: 2,
    });

    expect(Array.from(collectInitiallyExpandedPaths(tree))).toEqual([
      '$',
      '$.nested',
    ]);
  });

  it('finds case-insensitive key and value matches and expands ancestors', () => {
    const tree = buildPayloadTree({
      outer: {
        Temperature: 42,
        nested: {
          message: 'Hello world',
        },
      },
      plain: 'no match here',
    });

    const keyMatches = findPayloadMatches(tree, 'temp');
    expect(keyMatches.matches).toEqual([
      {
        id: '$.outer.Temperature:key',
        path: '$.outer.Temperature',
        target: 'key',
      },
    ]);
    expect(Array.from(keyMatches.expandedPaths)).toEqual(['$.outer', '$']);

    const valueMatches = findPayloadMatches(tree, 'hello');
    expect(valueMatches.matches).toEqual([
      {
        id: '$.outer.nested.message:value',
        path: '$.outer.nested.message',
        target: 'value',
      },
    ]);
    expect(Array.from(valueMatches.expandedPaths)).toEqual([
      '$.outer.nested',
      '$.outer',
      '$',
    ]);
  });

  it('returns no matches for empty queries or collection values', () => {
    const tree = buildPayloadTree({
      payload: {
        child: true,
      },
    });

    expect(findPayloadMatches(tree, '   ')).toEqual({
      expandedPaths: new Set<string>(),
      matches: [],
    });

    const noCollectionMatches = findPayloadMatches(tree, '[object object]');
    expect(noCollectionMatches.matches).toEqual([]);
    expect(noCollectionMatches.expandedPaths.size).toBe(0);
  });

  it('collects all expandable paths and accurate subtree counts', () => {
    const tree = buildPayloadTree({
      outer: {
        inner: [1, 2],
      },
    });

    expect(tree.subtreeNodeCount).toBe(5);
    expect(Array.from(collectExpandablePaths(tree))).toEqual([
      '$',
      '$.outer',
      '$.outer.inner',
    ]);
  });
});
