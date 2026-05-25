import { z } from "zod";

export const globalConfigSchema = z
  .object({
    default_workspace: z.string().trim().min(1).optional(),
    runtime_version: z.string().trim().min(1).optional(),
  })
  .passthrough();

export const projectPointerSchema = z
  .object({
    workspace: z.string().trim().min(1),
  })
  .passthrough();

export type GlobalConfig = z.infer<typeof globalConfigSchema>;
export type ProjectPointer = z.infer<typeof projectPointerSchema>;
