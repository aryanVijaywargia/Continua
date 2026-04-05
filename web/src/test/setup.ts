import '@testing-library/jest-dom';
import { cleanup } from '@testing-library/react';
import { afterEach, beforeEach } from 'vitest';
import {
  installMatchMediaMock,
  resetMatchMedia,
} from './matchMedia';

installMatchMediaMock();

class ResizeObserverMock {
  observe() {}

  unobserve() {}

  disconnect() {}
}

Object.defineProperty(window, 'ResizeObserver', {
  configurable: true,
  writable: true,
  value: ResizeObserverMock,
});
Object.defineProperty(globalThis, 'ResizeObserver', {
  configurable: true,
  writable: true,
  value: ResizeObserverMock,
});

beforeEach(() => {
  installMatchMediaMock();
  resetMatchMedia();
});

afterEach(() => {
  cleanup();
});
