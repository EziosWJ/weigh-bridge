import { useEffect } from "react";
import { connectMessages } from "@/lib/message-events";
import { useAuthStore } from "@/store/auth-store";

export function MessageConnections({ authenticated = false }: { authenticated?: boolean }) {
  const token = useAuthStore((state) => state.token);
  useEffect(() => connectMessages(null), []);
  useEffect(() => {
    if (authenticated && token) return connectMessages(token);
  }, [authenticated, token]);
  return null;
}
