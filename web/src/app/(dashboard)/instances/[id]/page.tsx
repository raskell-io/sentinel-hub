"use client";

import { use, useState } from "react";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import {
  ArrowLeft,
  Server,
  Tag,
  Activity,
  CheckCircle2,
  XCircle,
  RefreshCw,
} from "lucide-react";
import { formatDistanceToNow, format } from "date-fns";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { StatusBadge } from "@/components/instances/status-badge";
import { MetricsChart, MultiLineChart } from "@/components/metrics/metrics-chart";
import { PeriodSelector, Period } from "@/components/metrics/period-selector";
import { getInstance, getInstanceMetrics, TimeSeriesPoint } from "@/lib/api";
import { cn } from "@/lib/utils";

interface PageProps {
  params: Promise<{ id: string }>;
}

export default function InstanceDetailPage({ params }: PageProps) {
  const { id } = use(params);
  const [period, setPeriod] = useState<Period>("1h");

  const { data: instance, isLoading, error } = useQuery({
    queryKey: ["instance", id],
    queryFn: () => getInstance(id),
  });

  const {
    data: metricsData,
    isLoading: metricsLoading,
    refetch: refetchMetrics,
    isFetching: metricsFetching,
  } = useQuery({
    queryKey: ["instance-metrics", id, period],
    queryFn: () => getInstanceMetrics(id, { period }),
    enabled: !!instance,
    refetchInterval: 30000,
  });

  if (isLoading) {
    return (
      <div className="space-y-6">
        <Skeleton className="h-8 w-48" />
        <div className="grid gap-6 md:grid-cols-2">
          <Skeleton className="h-48" />
          <Skeleton className="h-48" />
        </div>
      </div>
    );
  }

  if (error || !instance) {
    return (
      <div className="space-y-6">
        <div>
          <Link href="/instances">
            <Button variant="ghost" size="sm">
              <ArrowLeft className="mr-2 h-4 w-4" />
              Back to Instances
            </Button>
          </Link>
        </div>
        <Card>
          <CardContent className="py-12 text-center">
            <Server className="mx-auto h-12 w-12 text-muted-foreground/50" />
            <h3 className="mt-4 text-lg font-semibold">Instance not found</h3>
            <p className="mt-2 text-sm text-muted-foreground">
              The requested instance could not be found.
            </p>
          </CardContent>
        </Card>
      </div>
    );
  }

  // Combine latency time series for multi-line chart
  const latencyData = metricsData
    ? combineTimeSeries({
        p50: metricsData.timeSeries.latencyP50,
        p95: metricsData.timeSeries.latencyP95,
        p99: metricsData.timeSeries.latencyP99,
      })
    : [];

  return (
    <div className="space-y-6">
      {/* Back button */}
      <div>
        <Link href="/instances">
          <Button variant="ghost" size="sm">
            <ArrowLeft className="mr-2 h-4 w-4" />
            Back to Instances
          </Button>
        </Link>
      </div>

      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-4">
          <div className="rounded-lg bg-primary/10 p-3">
            <Server className="h-6 w-6 text-primary" />
          </div>
          <div>
            <h1 className="text-3xl font-bold tracking-tight">{instance.name}</h1>
            <p className="text-muted-foreground font-mono">{instance.hostname}</p>
          </div>
        </div>
        <StatusBadge status={instance.status} />
      </div>

      {/* Metrics Section */}
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="text-xl font-semibold">Performance Metrics</h2>
          <div className="flex items-center gap-2">
            <PeriodSelector value={period} onChange={setPeriod} />
            <Button
              variant="outline"
              size="sm"
              onClick={() => refetchMetrics()}
              disabled={metricsFetching}
            >
              <RefreshCw
                className={`h-4 w-4 ${metricsFetching ? "animate-spin" : ""}`}
              />
            </Button>
          </div>
        </div>

        {/* Metrics Summary Cards */}
        {metricsLoading ? (
          <div className="grid gap-4 md:grid-cols-4">
            {[...Array(4)].map((_, i) => (
              <Skeleton key={i} className="h-24" />
            ))}
          </div>
        ) : metricsData ? (
          <div className="grid gap-4 md:grid-cols-4">
            <MetricCard
              title="Requests/sec"
              value={metricsData.summary.requestsPerSecond.toFixed(1)}
              subValue={`${formatNumber(metricsData.summary.totalRequests)} total`}
            />
            <MetricCard
              title="Error Rate"
              value={`${metricsData.summary.errorRate.toFixed(2)}%`}
              subValue={`${formatNumber(metricsData.summary.totalErrors)} errors`}
              highlight={metricsData.summary.errorRate > 1}
            />
            <MetricCard
              title="P95 Latency"
              value={formatLatency(metricsData.summary.p95LatencyMs)}
              subValue={`P50: ${formatLatency(metricsData.summary.p50LatencyMs)}`}
              highlight={metricsData.summary.p95LatencyMs > 500}
            />
            <MetricCard
              title="Active Connections"
              value={metricsData.summary.activeConnections.toString()}
              subValue={formatBytes(metricsData.summary.bytesOut) + " out"}
            />
          </div>
        ) : (
          <div className="grid gap-4 md:grid-cols-4">
            {[...Array(4)].map((_, i) => (
              <Card key={i}>
                <CardContent className="pt-6">
                  <div className="text-2xl font-bold text-muted-foreground">--</div>
                  <p className="text-xs text-muted-foreground">No data</p>
                </CardContent>
              </Card>
            ))}
          </div>
        )}

        {/* Charts */}
        <div className="grid gap-6 md:grid-cols-2">
          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-base">Request Rate</CardTitle>
            </CardHeader>
            <CardContent>
              {metricsLoading ? (
                <Skeleton className="h-[180px]" />
              ) : metricsData ? (
                <MetricsChart
                  data={metricsData.timeSeries.requests}
                  color="hsl(var(--primary))"
                  formatValue={(v) => `${v.toFixed(0)}/s`}
                  height={180}
                />
              ) : (
                <ChartPlaceholder />
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-base">Latency Percentiles</CardTitle>
            </CardHeader>
            <CardContent>
              {metricsLoading ? (
                <Skeleton className="h-[180px]" />
              ) : metricsData ? (
                <MultiLineChart
                  data={latencyData}
                  lines={[
                    { dataKey: "p50", color: "hsl(142, 76%, 36%)", name: "P50" },
                    { dataKey: "p95", color: "hsl(48, 96%, 53%)", name: "P95" },
                    { dataKey: "p99", color: "hsl(0, 84%, 60%)", name: "P99" },
                  ]}
                  formatValue={(v) => `${v.toFixed(0)}ms`}
                  height={180}
                />
              ) : (
                <ChartPlaceholder />
              )}
            </CardContent>
          </Card>
        </div>

        {/* Upstreams Health */}
        {metricsData && metricsData.upstreams.length > 0 && (
          <Card>
            <CardHeader>
              <CardTitle className="text-base flex items-center gap-2">
                <Activity className="h-4 w-4" />
                Upstream Health
              </CardTitle>
              <CardDescription>Status of backend upstreams</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="grid gap-3 md:grid-cols-2 lg:grid-cols-3">
                {metricsData.upstreams.map((upstream) => (
                  <div
                    key={upstream.name}
                    className="flex items-center justify-between rounded-lg border p-3"
                  >
                    <div className="flex items-center gap-2">
                      {upstream.healthy ? (
                        <CheckCircle2 className="h-4 w-4 text-green-500" />
                      ) : (
                        <XCircle className="h-4 w-4 text-red-500" />
                      )}
                      <div>
                        <p className="font-medium text-sm">{upstream.name}</p>
                        <p className="text-xs text-muted-foreground">
                          {upstream.activeConnections} conns
                        </p>
                      </div>
                    </div>
                    <div className="text-right">
                      <p className="text-sm font-medium">
                        {formatLatency(upstream.avgLatencyMs)}
                      </p>
                      <p className="text-xs text-muted-foreground">
                        {upstream.totalErrors > 0 && (
                          <span className="text-red-500">
                            {upstream.totalErrors} errors
                          </span>
                        )}
                      </p>
                    </div>
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>
        )}
      </div>

      {/* Details grid */}
      <div className="grid gap-6 md:grid-cols-2">
        {/* Instance Info */}
        <Card>
          <CardHeader>
            <CardTitle>Instance Details</CardTitle>
            <CardDescription>Core information about this instance</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid gap-4">
              <div>
                <p className="text-sm font-medium text-muted-foreground">ID</p>
                <p className="font-mono text-sm">{instance.id}</p>
              </div>
              <div>
                <p className="text-sm font-medium text-muted-foreground">
                  Agent Version
                </p>
                <p>{instance.agentVersion}</p>
              </div>
              <div>
                <p className="text-sm font-medium text-muted-foreground">
                  Sentinel Version
                </p>
                <p>{instance.sentinelVersion}</p>
              </div>
              <div>
                <p className="text-sm font-medium text-muted-foreground">
                  Last Seen
                </p>
                <p>
                  {formatDistanceToNow(new Date(instance.lastSeenAt), {
                    addSuffix: true,
                  })}
                </p>
              </div>
              <div>
                <p className="text-sm font-medium text-muted-foreground">
                  Registered
                </p>
                <p>
                  {format(new Date(instance.createdAt), "PPpp")}
                </p>
              </div>
            </div>
          </CardContent>
        </Card>

        {/* Configuration */}
        <Card>
          <CardHeader>
            <CardTitle>Configuration</CardTitle>
            <CardDescription>Current configuration assignment</CardDescription>
          </CardHeader>
          <CardContent>
            {instance.currentConfigId ? (
              <div className="space-y-4">
                <div>
                  <p className="text-sm font-medium text-muted-foreground">
                    Config ID
                  </p>
                  <Link
                    href={`/configs/${instance.currentConfigId}`}
                    className="font-mono text-sm text-primary hover:underline"
                  >
                    {instance.currentConfigId}
                  </Link>
                </div>
                <div>
                  <p className="text-sm font-medium text-muted-foreground">
                    Version
                  </p>
                  <p>v{instance.currentConfigVersion}</p>
                </div>
              </div>
            ) : (
              <div className="py-8 text-center">
                <p className="text-muted-foreground">
                  No configuration assigned
                </p>
                <Button variant="outline" size="sm" className="mt-4" disabled>
                  Assign Configuration
                </Button>
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      {/* Labels */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Tag className="h-4 w-4" />
            Labels
          </CardTitle>
          <CardDescription>
            Labels used for filtering and deployment targeting
          </CardDescription>
        </CardHeader>
        <CardContent>
          {Object.keys(instance.labels).length > 0 ? (
            <div className="flex flex-wrap gap-2">
              {Object.entries(instance.labels).map(([key, value]) => (
                <Badge key={key} variant="secondary">
                  {key}={value}
                </Badge>
              ))}
            </div>
          ) : (
            <p className="text-muted-foreground text-sm">No labels assigned</p>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function MetricCard({
  title,
  value,
  subValue,
  highlight = false,
}: {
  title: string;
  value: string;
  subValue: string;
  highlight?: boolean;
}) {
  return (
    <Card>
      <CardContent className="pt-6">
        <p className="text-sm font-medium text-muted-foreground">{title}</p>
        <p
          className={cn(
            "text-2xl font-bold",
            highlight && "text-yellow-500"
          )}
        >
          {value}
        </p>
        <p className="text-xs text-muted-foreground">{subValue}</p>
      </CardContent>
    </Card>
  );
}

function ChartPlaceholder() {
  return (
    <div className="flex items-center justify-center rounded-lg border border-dashed h-[180px]">
      <p className="text-sm text-muted-foreground">No metrics data available</p>
    </div>
  );
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

function combineTimeSeries(
  series: Record<string, TimeSeriesPoint[]>
): Array<Record<string, string | number>> {
  const keys = Object.keys(series);
  if (keys.length === 0) return [];

  const firstSeries = series[keys[0]];
  return firstSeries.map((point, index) => {
    const combined: Record<string, string | number> = {
      timestamp: point.timestamp,
    };
    keys.forEach((key) => {
      combined[key] = series[key][index]?.value ?? 0;
    });
    return combined;
  });
}
