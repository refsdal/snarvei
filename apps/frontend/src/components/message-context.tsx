import { createContext, useContext, useMemo, useState } from "react";

export type AppMessage = { severity: "success" | "error" | "info"; text: string };

type MessageContextValue = { message: AppMessage | null; setMessage: (message: AppMessage | null) => void };

const MessageContext = createContext<MessageContextValue | null>(null);

export function MessageProvider({ children }: { children: React.ReactNode }) {
  const [message, setMessage] = useState<AppMessage | null>(null);
  const value = useMemo(() => ({ message, setMessage }), [message]);
  return <MessageContext.Provider value={value}>{children}</MessageContext.Provider>;
}

export function useMessage(): MessageContextValue {
  const context = useContext(MessageContext);
  if (!context) {
    throw new Error("useMessage must be used within a MessageProvider");
  }
  return context;
}
