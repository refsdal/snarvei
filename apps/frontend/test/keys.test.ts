import { describe, expect, test } from "bun:test";
import { keys } from "../src/lib/data/keys";

describe("query keys", () => {
  test("match the spec's shapes", () => {
    expect(keys.me).toEqual(["me"]);
    expect(keys.config).toEqual(["config"]);
    expect(keys.organizations).toEqual(["organizations"]);
    expect(keys.sessions).toEqual(["sessions"]);
    expect(keys.teams("o1")).toEqual(["teams", "o1"]);
    expect(keys.teamMembers("t1")).toEqual(["teamMembers", "t1"]);
    expect(keys.members("o1")).toEqual(["members", "o1"]);
    expect(keys.invitations("o1")).toEqual(["invitations", "o1"]);
    expect(keys.invitation("i1")).toEqual(["invitation", "i1"]);
    expect(keys.links("o1", { page: 2, pageSize: 100, teamId: "t1" })).toEqual([
      "links",
      "o1",
      { page: 2, pageSize: 100, teamId: "t1" },
    ]);
    expect(keys.link("l1")).toEqual(["link", "l1"]);
    expect(keys.history("l1", 1)).toEqual(["history", "l1", 1]);
    expect(keys.analytics("l1", 30)).toEqual(["analytics", "l1", 30]);
  });

  test("a link key is a prefix of its history and analytics keys' link scope", () => {
    // invalidating ["links", orgId] must catch every filter variant
    expect(keys.links("o1", { page: 1, pageSize: 100 }).slice(0, 2)).toEqual(["links", "o1"]);
  });
});
