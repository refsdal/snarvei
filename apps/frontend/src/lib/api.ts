import createClient from "openapi-fetch";
import type { paths } from "./api-schema";

// "" = same origin: the Go server serves the SPA and the API together, and
// in development Vite proxies the server-owned paths to :3000.
export const client = createClient<paths>({ baseUrl: "", credentials: "include" });

export class ApiError extends Error {
  status: number;
  code: string;
  details?: Record<string, unknown>;
  constructor(status: number, code: string, message: string, details?: Record<string, unknown>) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.details = details;
  }
}

interface OpenApiFetchResult {
  data?: unknown;
  error?: unknown;
  response: Response;
}

type Envelope = { code?: string; message?: string; details?: Record<string, unknown> };

function toApiError(status: number, body: unknown): ApiError {
  const env = (body ?? {}) as Envelope;
  return new ApiError(status, env.code ?? "INTERNAL", env.message ?? `Request failed (${status})`, env.details);
}

// openapi-fetch resolves { data, error, response } instead of throwing; unwrap
// turns that (or a raw Response, for the multipart routes that bypass the
// typed client) into "return data or throw ApiError".
export async function unwrap<T = unknown>(
  result: OpenApiFetchResult | Response | Promise<OpenApiFetchResult | Response>,
): Promise<T> {
  const r = await result;
  if (r instanceof Response) {
    if (r.status === 204) return undefined as T;
    let body: unknown = null;
    try {
      body = await r.json();
    } catch {
      body = null;
    }
    if (!r.ok) throw toApiError(r.status, body);
    return body as T;
  }
  if (r.error !== undefined || !r.response.ok) throw toApiError(r.response.status, r.error);
  if (r.response.status === 204) return undefined as T;
  return r.data as T;
}

export function errorMessage(err: unknown, fallback: string): string {
  if (err instanceof Error && err.message) return err.message;
  return fallback;
}
