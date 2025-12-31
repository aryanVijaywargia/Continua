export interface ToolConfig {
  name: string;
  description: string;
  parameters?: Record<string, unknown>;
}

export function tool(config: ToolConfig) {
  return function (
    target: unknown,
    propertyKey: string,
    descriptor: PropertyDescriptor
  ) {
    // Store tool metadata
    const originalMethod = descriptor.value;
    descriptor.value = async function (...args: unknown[]) {
      // TODO: Add recording/replay logic
      return originalMethod.apply(this, args);
    };
    return descriptor;
  };
}
