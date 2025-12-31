import { cn } from "@/lib/utils";
import { Badge } from "@/components/ui/badge";

interface StatusBadgeProps {
  status: "online" | "offline" | "unhealthy";
}

export function StatusBadge({ status }: StatusBadgeProps) {
  return (
    <Badge
      variant="outline"
      className={cn(
        "capitalize",
        status === "online" && "border-green-500 bg-green-500/10 text-green-700 dark:text-green-400",
        status === "offline" && "border-gray-500 bg-gray-500/10 text-gray-700 dark:text-gray-400",
        status === "unhealthy" && "border-yellow-500 bg-yellow-500/10 text-yellow-700 dark:text-yellow-400"
      )}
    >
      <span
        className={cn(
          "mr-1.5 h-2 w-2 rounded-full",
          status === "online" && "bg-green-500",
          status === "offline" && "bg-gray-500",
          status === "unhealthy" && "bg-yellow-500"
        )}
      />
      {status}
    </Badge>
  );
}
