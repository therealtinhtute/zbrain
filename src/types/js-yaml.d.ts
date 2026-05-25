declare module "js-yaml" {
  export function load(contents: string): unknown;
  export function dump(value: unknown, options?: Record<string, unknown>): string;

  const YAML: {
    load: typeof load;
    dump: typeof dump;
  };

  export default YAML;
}
