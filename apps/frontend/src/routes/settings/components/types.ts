import type { AppMessage } from "../../../types";

export type ActionRunner = (action: string, work: () => Promise<void>) => Promise<void>;

export type SharedSectionProps = {
  busyAction: string | null;
  setMessage: (message: AppMessage) => void;
  refreshSessionState: () => Promise<void>;
  runAction: ActionRunner;
};
