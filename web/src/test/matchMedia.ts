import { vi } from 'vitest';

type MediaQueryListener = (event: MediaQueryListEvent) => void;

let matches = true;
const mediaQueryLists = new Set<MockMediaQueryList>();

export function installMatchMediaMock() {
  Object.defineProperty(window, 'matchMedia', {
    configurable: true,
    writable: true,
    value: vi.fn().mockImplementation((query: string) => {
      const mediaQueryList = createMediaQueryList(query);
      mediaQueryLists.add(mediaQueryList);
      return mediaQueryList;
    }),
  });
}

export function resetMatchMedia() {
  matches = true;

  for (const mediaQueryList of mediaQueryLists) {
    mediaQueryList.matches = matches;
  }
}

export function setMatchMediaMatches(nextMatches: boolean) {
  matches = nextMatches;

  for (const mediaQueryList of mediaQueryLists) {
    mediaQueryList.matches = nextMatches;
    mediaQueryList.dispatch();
  }
}

interface MockMediaQueryList extends MediaQueryList {
  dispatch: () => void;
  matches: boolean;
}

function createMediaQueryList(query: string): MockMediaQueryList {
  const listeners = new Set<MediaQueryListener>();

  return {
    get matches() {
      return matches;
    },
    set matches(_value: boolean) {},
    media: query,
    onchange: null,
    addEventListener: (_type: string, listener: EventListenerOrEventListenerObject) => {
      if (typeof listener === 'function') {
        listeners.add(listener as MediaQueryListener);
      }
    },
    removeEventListener: (_type: string, listener: EventListenerOrEventListenerObject) => {
      if (typeof listener === 'function') {
        listeners.delete(listener as MediaQueryListener);
      }
    },
    addListener: (listener: MediaQueryListener) => {
      listeners.add(listener);
    },
    removeListener: (listener: MediaQueryListener) => {
      listeners.delete(listener);
    },
    dispatchEvent: () => true,
    dispatch: () => {
      const event = { matches, media: query } as MediaQueryListEvent;
      for (const listener of listeners) {
        listener(event);
      }
    },
  };
}
