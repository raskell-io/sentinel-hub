"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { Loader2 } from "lucide-react";
import { useAuthStore } from "@/stores/auth-store";
import { getCurrentUser, ApiError } from "@/lib/api";

export function AuthGuard({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const [isChecking, setIsChecking] = useState(true);
  const { accessToken, isAuthenticated, setAuth, clearAuth } = useAuthStore();

  useEffect(() => {
    async function checkAuth() {
      // No token, redirect to login
      if (!accessToken) {
        router.replace("/login");
        return;
      }

      // Token exists, verify it's still valid
      try {
        const user = await getCurrentUser();
        setAuth(user, accessToken);
        setIsChecking(false);
      } catch (err) {
        if (err instanceof ApiError && err.status === 401) {
          clearAuth();
          router.replace("/login");
        } else {
          // Network error or other issue, still allow if we have a token
          setIsChecking(false);
        }
      }
    }

    checkAuth();
  }, [accessToken, router, setAuth, clearAuth]);

  if (isChecking) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-primary" />
      </div>
    );
  }

  if (!isAuthenticated) {
    return null;
  }

  return <>{children}</>;
}
