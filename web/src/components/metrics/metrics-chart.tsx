"use client";

import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  Legend,
} from "recharts";
import { format } from "date-fns";
import { TimeSeriesPoint } from "@/lib/api";

interface MetricsChartProps {
  data: TimeSeriesPoint[];
  dataKey?: string;
  color?: string;
  title?: string;
  yAxisLabel?: string;
  formatValue?: (value: number) => string;
  height?: number;
}

export function MetricsChart({
  data,
  dataKey = "value",
  color = "hsl(var(--primary))",
  title,
  yAxisLabel,
  formatValue = (v) => v.toLocaleString(),
  height = 200,
}: MetricsChartProps) {
  const chartData = data.map((point) => ({
    timestamp: new Date(point.timestamp).getTime(),
    [dataKey]: point.value,
  }));

  return (
    <div className="w-full" style={{ height }}>
      {title && (
        <h4 className="text-sm font-medium mb-2 text-muted-foreground">
          {title}
        </h4>
      )}
      <ResponsiveContainer width="100%" height="100%">
        <LineChart data={chartData}>
          <CartesianGrid strokeDasharray="3 3" className="stroke-muted" />
          <XAxis
            dataKey="timestamp"
            type="number"
            domain={["dataMin", "dataMax"]}
            tickFormatter={(ts) => format(new Date(ts), "HH:mm")}
            className="text-xs"
            stroke="hsl(var(--muted-foreground))"
            fontSize={12}
          />
          <YAxis
            tickFormatter={formatValue}
            className="text-xs"
            stroke="hsl(var(--muted-foreground))"
            fontSize={12}
            label={
              yAxisLabel
                ? {
                    value: yAxisLabel,
                    angle: -90,
                    position: "insideLeft",
                    style: { fontSize: 12, fill: "hsl(var(--muted-foreground))" },
                  }
                : undefined
            }
          />
          <Tooltip
            content={({ active, payload }) => {
              if (!active || !payload?.length) return null;
              const point = payload[0];
              return (
                <div className="rounded-lg border bg-background p-2 shadow-sm">
                  <p className="text-xs text-muted-foreground">
                    {format(new Date(point.payload.timestamp), "PPpp")}
                  </p>
                  <p className="text-sm font-medium">
                    {formatValue(point.value as number)}
                  </p>
                </div>
              );
            }}
          />
          <Line
            type="monotone"
            dataKey={dataKey}
            stroke={color}
            strokeWidth={2}
            dot={false}
            activeDot={{ r: 4 }}
          />
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
}

interface MultiLineChartProps {
  data: Array<Record<string, string | number>>;
  lines: Array<{
    dataKey: string;
    color: string;
    name: string;
  }>;
  title?: string;
  yAxisLabel?: string;
  formatValue?: (value: number) => string;
  height?: number;
}

export function MultiLineChart({
  data,
  lines,
  title,
  yAxisLabel,
  formatValue = (v) => v.toLocaleString(),
  height = 200,
}: MultiLineChartProps) {
  const chartData = data.map((point) => ({
    ...point,
    timestamp: new Date(point.timestamp).getTime(),
  }));

  return (
    <div className="w-full" style={{ height }}>
      {title && (
        <h4 className="text-sm font-medium mb-2 text-muted-foreground">
          {title}
        </h4>
      )}
      <ResponsiveContainer width="100%" height="100%">
        <LineChart data={chartData}>
          <CartesianGrid strokeDasharray="3 3" className="stroke-muted" />
          <XAxis
            dataKey="timestamp"
            type="number"
            domain={["dataMin", "dataMax"]}
            tickFormatter={(ts) => format(new Date(ts), "HH:mm")}
            className="text-xs"
            stroke="hsl(var(--muted-foreground))"
            fontSize={12}
          />
          <YAxis
            tickFormatter={formatValue}
            className="text-xs"
            stroke="hsl(var(--muted-foreground))"
            fontSize={12}
            label={
              yAxisLabel
                ? {
                    value: yAxisLabel,
                    angle: -90,
                    position: "insideLeft",
                    style: { fontSize: 12, fill: "hsl(var(--muted-foreground))" },
                  }
                : undefined
            }
          />
          <Tooltip
            content={({ active, payload }) => {
              if (!active || !payload?.length) return null;
              const ts = payload[0]?.payload?.timestamp;
              return (
                <div className="rounded-lg border bg-background p-2 shadow-sm">
                  <p className="text-xs text-muted-foreground mb-1">
                    {format(new Date(ts), "PPpp")}
                  </p>
                  {payload.map((entry) => (
                    <p
                      key={entry.dataKey}
                      className="text-sm"
                      style={{ color: entry.color }}
                    >
                      {entry.name}: {formatValue(entry.value as number)}
                    </p>
                  ))}
                </div>
              );
            }}
          />
          <Legend
            wrapperStyle={{ fontSize: 12 }}
            iconType="line"
          />
          {lines.map((line) => (
            <Line
              key={line.dataKey}
              type="monotone"
              dataKey={line.dataKey}
              stroke={line.color}
              name={line.name}
              strokeWidth={2}
              dot={false}
              activeDot={{ r: 4 }}
            />
          ))}
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
}
