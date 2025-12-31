"use client";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

type Period = "1h" | "6h" | "24h" | "7d";

interface PeriodSelectorProps {
  value: Period;
  onChange: (period: Period) => void;
}

const periods: { value: Period; label: string }[] = [
  { value: "1h", label: "1H" },
  { value: "6h", label: "6H" },
  { value: "24h", label: "24H" },
  { value: "7d", label: "7D" },
];

export function PeriodSelector({ value, onChange }: PeriodSelectorProps) {
  return (
    <div className="flex items-center gap-1 rounded-lg border p-1">
      {periods.map((period) => (
        <Button
          key={period.value}
          variant="ghost"
          size="sm"
          className={cn(
            "h-7 px-3 text-xs",
            value === period.value && "bg-primary text-primary-foreground hover:bg-primary/90"
          )}
          onClick={() => onChange(period.value)}
        >
          {period.label}
        </Button>
      ))}
    </div>
  );
}

export type { Period };
