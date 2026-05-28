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

export const projectPointerSchema = z
  .object({
    workspace: z.string().trim().min(1),
    secondary_workspaces: z.array(secondaryWorkspaceEntrySchema).optional(),
  })
  .passthrough();

export type GlobalConfig = z.infer<typeof globalConfigSchema>;
export type ProjectPointer = z.infer<typeof projectPointerSchema>;
export type SecondaryWorkspaceEntry = z.infer<typeof secondaryWorkspaceEntrySchema>;
