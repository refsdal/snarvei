/**
 * API DTO types shared by the Worker and the React app. They are inferred from
 * the zod/OpenAPI schemas so the client can never drift from the server.
 * Type-only: this file must not import runtime code from the Worker.
 */
import type { z } from "@hono/zod-openapi";
import type { AnalyticsSummarySchema, HistoryItemSchema, LinkSchema, TeamMemberSchema } from "../worker/openapi/schemas";

export type LinkDto = z.infer<typeof LinkSchema>;
export type HistoryItemDto = z.infer<typeof HistoryItemSchema>;
export type AnalyticsSummaryDto = z.infer<typeof AnalyticsSummarySchema>;
export type TeamMemberDto = z.infer<typeof TeamMemberSchema>;
