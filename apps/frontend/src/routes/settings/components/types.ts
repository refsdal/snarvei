import type { AppMessage } from "../../../components/message-context";
import type { Me } from "../../../lib/data";

export type ActionRunner = (action: string, work: () => Promise<void>) => Promise<void>;

export type SharedSectionProps = {
  me: Me;
  busyAction: string | null;
  setMessage: (message: AppMessage) => void;
  runAction: ActionRunner;
};
