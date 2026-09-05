import { describe, expect, test } from "bun:test";
import { ApiError, errorMessage, unwrap } from "../src/lib/api";

const json = (status: number, body: unknown) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

describe("unwrap", () => {
  test("returns data from an openapi-fetch result", async () => {
    const out = await unwrap<{ ok: boolean }>({ data: { ok: true }, response: json(200, { ok: true }) });
    expect(out).toEqual({ ok: true });
  });

  test("throws ApiError with the envelope's code and message", async () => {
    const err = await unwrap<never>({
      error: { code: "SLUG_TAKEN", message: "That slug is already taken" },
      response: json(409, {}),
    }).catch((e) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err.status).toBe(409);
    expect(err.code).toBe("SLUG_TAKEN");
    expect(err.message).toBe("That slug is already taken");
  });

  test("accepts a raw Response and parses the envelope", async () => {
    const err = await unwrap<never>(
      json(400, { code: "VALIDATION_FAILED", message: "bad", details: { fields: { a: "x" } } }),
    ).catch((e) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err.details).toEqual({ fields: { a: "x" } });
    const ok = await unwrap<{ n: number }>(json(200, { n: 1 }));
    expect(ok).toEqual({ n: 1 });
  });

  test("a 204 resolves to undefined", async () => {
    expect(await unwrap({ response: new Response(null, { status: 204 }) })).toBeUndefined();
  });

  test("a non-JSON failure becomes a generic ApiError", async () => {
    const err = await unwrap<never>(new Response("boom", { status: 502 })).catch((e) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err.status).toBe(502);
    expect(err.code).toBe("INTERNAL");
  });
});

describe("errorMessage", () => {
  test("prefers the error's message and falls back", () => {
    expect(errorMessage(new ApiError(404, "NOT_FOUND", "Link not found"), "x")).toBe("Link not found");
    expect(errorMessage("nope", "fallback")).toBe("fallback");
  });
});
