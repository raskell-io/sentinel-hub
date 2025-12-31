"use client";

import { Activity, AlertTriangle, Clock, Gauge, ArrowUp, ArrowDown } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { MetricsSummary as MetricsSummaryType } from "@/lib/api";
import { cn } from "@/lib/utils";

interface MetricsSummaryProps {
  data: MetricsSummaryType;
  previousData?: MetricsSummaryType;
}

function formatNumber(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return n.toFixed(0);
}

function formatLatency(ms: number): string {
  if (ms >= 1000) return `${(ms / 1000).toFixed(2)}s`;
  return `${ms.toFixed(0)}ms`;
}

function formatBytes(bytes: number): string {
  if (bytes >= 1_000_000_000) return `${(bytes / 1_000_000_000).toFixed(1)} GB`;
  if (bytes >= 1_000_000) return `${(bytes / 1_000_000).toFixed(1)} MB`;
  if (bytes >= 1_000) return `${(bytes / 1_000).toFixed(1)} KB`;
  return `${bytes} B`;
}

function ChangeIndicator({ current, previous }: { current: number; previous?: number }) {
  if (previous === undefined || previous === 0) return null;

  const change = ((current - previous) / previous) * 100;
  const isPositive = change > 0;
  const isSignificant = Math.abs(change) >= 1;

  if (!isSignificant) return null;

  return (
    <span
      className={cn(
        "flex items-center text-xs ml-2",
        isPositive ? "text-red-500" : "text-green-500"
      )}
    >
      {isPositive ? <ArrowUp className="h-3 w-3" /> : <ArrowDown className="h-3 w-3" />}
      {Math.abs(change).toFixed(1)}%
    </span>
  );
}

export function MetricsSummaryCards({ data, previousData }: MetricsSummaryProps) {
  return (
    <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
      <Card>
        <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
          <CardTitle className="text-sm font-medium">Requests/sec</CardTitle>
          <Activity className="h-4 w-4 text-muted-foreground" />
        </CardHeader>
        <CardContent>
          <div className="flex items-center">
            <div className="text-2xl font-bold">
              {formatNumber(data.requestsPerSecond)}
            </div>
            <ChangeIndicator
              current={data.requestsPerSecond}
              previous={previousData?.requestsPerSecond}
            />
          </div>
          <p className="text-xs text-muted-foreground">
            {formatNumber(data.totalRequests)} total requests
          </p>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
          <CardTitle className="text-sm font-medium">Error Rate</CardTitle>
          <AlertTriangle className="h-4 w-4 text-muted-foreground" />
        </CardHeader>
        <CardContent>
          <div className="flex items-center">
            <div
              className={cn(
                "text-2xl font-bold",
                data.errorRate > 5 ? "text-red-500" : data.errorRate > 1 ? "text-yellow-500" : ""
              )}
            >
              {data.errorRate.toFixed(2)}%
            </div>
            <ChangeIndicator
              current={data.errorRate}
              previous={previousData?.errorRate}
            />
          </div>
          <p className="text-xs text-muted-foreground">
            {formatNumber(data.totalErrors)} errors
          </p>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
          <CardTitle className="text-sm font-medium">P95 Latency</CardTitle>
          <Clock className="h-4 w-4 text-muted-foreground" />
        </CardHeader>
        <CardContent>
          <div className="flex items-center">
            <div
              className={cn(
                "text-2xl font-bold",
                data.p95LatencyMs > 1000 ? "text-red-500" : data.p95LatencyMs > 500 ? "text-yellow-500" : ""
              )}
            >
              {formatLatency(data.p95LatencyMs)}
            </div>
            <ChangeIndicator
              current={data.p95LatencyMs}
              previous={previousData?.p95LatencyMs}
            />
          </div>
          <p className="text-xs text-muted-foreground">
            P50: {formatLatency(data.p50LatencyMs)} / P99: {formatLatency(data.p99LatencyMs)}
          </p>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
          <CardTitle className="text-sm font-medium">Throughput</CardTitle>
          <Gauge className="h-4 w-4 text-muted-foreground" />
        </CardHeader>
        <CardContent>
          <div className="text-2xl font-bold">
            {formatBytes(data.bytesOut)}
          </div>
          <p className="text-xs text-muted-foreground">
            {formatBytes(data.bytesIn)} in / {data.activeConnections} conns
          </p>
        </CardContent>
      </Card>
    </div>
  );
}
