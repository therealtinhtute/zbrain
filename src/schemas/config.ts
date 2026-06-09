import { z } from "zod";

export const globalConfigSchema = z
  .object({
    default_workspace: z.string().trim().min(1).optional(),
    runtime_version: z.string().trim().min(1).optional(),
  })
  .passthrough();

export const secondaryWorkspaceEntrySchema = z.object({
  workspace: z.string().trim().min(1),
  keywords: z.array(z.string().trim().min(1)).min(1),
  limit: z.number().int().positive().optional().default(3),
});

export const runtimeNameSchema = z.enum(["claude", "codex"]);

export const projectPointerSchema = z
  .object({
    workspace: z.string().trim().min(1),
    secondary_workspaces: z.array(secondaryWorkspaceEntrySchema).optional(),
  })
  .passthrough();

export const projectBindingSchema = z
  .object({
    project_root: z.string().trim().min(1),
    workspace: z.string().trim().min(1),
    context_file: z.string().trim().min(1),
    runtimes: z.array(runtimeNameSchema).optional(),
    secondary_workspaces: z.array(secondaryWorkspaceEntrySchema).optional(),
  })
  .passthrough();

export const projectRegistrySchema = z
  .object({
    projects: z.array(projectBindingSchema).default([]),
  })
  .passthrough();

export type GlobalConfig = z.infer<typeof globalConfigSchema>;
export type ProjectPointer = z.infer<typeof projectPointerSchema>;
export type SecondaryWorkspaceEntry = z.infer<typeof secondaryWorkspaceEntrySchema>;
export type RuntimeName = z.infer<typeof runtimeNameSchema>;
export type ProjectBinding = z.infer<typeof projectBindingSchema>;
export type ProjectRegistry = z.infer<typeof projectRegistrySchema>;
