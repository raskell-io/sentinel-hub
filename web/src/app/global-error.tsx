"use client";

import { AlertTriangle, RefreshCw } from "lucide-react";

export default function GlobalError({
  error,
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) {
  return (
    <html>
      <body>
        <div className="min-h-screen flex items-center justify-center bg-background p-4">
          <div className="max-w-md w-full text-center">
            <div className="mx-auto mb-6 rounded-full bg-red-100 p-4 w-fit">
              <AlertTriangle className="h-8 w-8 text-red-600" />
            </div>
            <h1 className="text-2xl font-bold mb-2">Application Error</h1>
            <p className="text-muted-foreground mb-6">
              A critical error occurred. Please refresh the page to try again.
            </p>
            {process.env.NODE_ENV === "development" && (
              <div className="rounded-md bg-muted p-4 mb-6 text-left font-mono text-xs overflow-auto max-h-40">
                <p className="font-semibold mb-2">Error Details:</p>
                <p>{error.message}</p>
                {error.digest && (
                  <p className="mt-2 text-muted-foreground">Digest: {error.digest}</p>
                )}
              </div>
            )}
            <button
              onClick={reset}
              className="inline-flex items-center justify-center rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90"
            >
              <RefreshCw className="mr-2 h-4 w-4" />
              Reload Application
            </button>
          </div>
        </div>
      </body>
    </html>
  );
}
