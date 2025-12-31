import { cn } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";
import { Loader2 } from "lucide-react";

type DeploymentStatus = "pending" | "in-progress" | "completed" | "failed" | "cancelled";

interface DeploymentStatusBadgeProps {
  status: DeploymentStatus;
}

const statusConfig: Record<DeploymentStatus, { label: string; className: string; showSpinner?: boolean }> = {
  pending: {
    label: "Pending",
    className: "border-gray-500 bg-gray-500/10 text-gray-700 dark:text-gray-400",
  },
  "in-progress": {
    label: "In Progress",
    className: "border-blue-500 bg-blue-500/10 text-blue-700 dark:text-blue-400",
    showSpinner: true,
  },
  completed: {
    label: "Completed",
    className: "border-green-500 bg-green-500/10 text-green-700 dark:text-green-400",
  },
  failed: {
    label: "Failed",
    className: "border-red-500 bg-red-500/10 text-red-700 dark:text-red-400",
  },
  cancelled: {
    label: "Cancelled",
    className: "border-yellow-500 bg-yellow-500/10 text-yellow-700 dark:text-yellow-400",
  },
};

export function DeploymentStatusBadge({ status }: DeploymentStatusBadgeProps) {
  const config = statusConfig[status];

  return (
    <Badge variant="outline" className={cn("gap-1", config.className)}>
      {config.showSpinner && <Loader2 className="h-3 w-3 animate-spin" />}
      {config.label}
    </Badge>
  );
}
