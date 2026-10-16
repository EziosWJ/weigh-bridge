import { useEffect, useState, type PropsWithChildren } from "react";
import { getCurrentUser } from "@/api/auth";
import { Button } from "@/components/ui/button";

export function RequireMessageAdmin({ children, resource }: PropsWithChildren<{ resource: string }>) {
  const [allowed, setAllowed] = useState<boolean | null>(null);
  const [failed, setFailed] = useState(false);
  const [retry, setRetry] = useState(0);
  useEffect(() => {
    let active = true;
    void getCurrentUser().then((user) => { if (active) { setAllowed(user.roles.some((role) => role.roleCode === "ADMIN")); setFailed(false); } }).catch(() => { if (active) setFailed(true); });
    return () => { active = false; };
  }, [retry]);
  if (failed) return <div role="alert">权限加载失败。<Button variant="secondary" onClick={() => setRetry(retry + 1)}>重试</Button></div>;
  if (allowed === null) return <p>正在检查权限…</p>;
  if (!allowed) return <p role="alert">无权限管理{resource}</p>;
  return children;
}
