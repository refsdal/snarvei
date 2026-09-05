// Owner is granted at organization creation and never assignable through the
// invite UI — only member/admin can be invited (see the createInvitation
// request body in lib/api-schema.d.ts).
export type InvitationRole = "member" | "admin";
