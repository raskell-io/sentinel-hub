"use client";

import { use } from "react";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { ArrowLeft, Server, Tag } from "lucide-react";
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
import { getInstance } from "@/lib/api";

interface PageProps {
  params: Promise<{ id: string }>;
}

export default function InstanceDetailPage({ params }: PageProps) {
  const { id } = use(params);

  const { data: instance, isLoading, error } = useQuery({
    queryKey: ["instance", id],
    queryFn: () => getInstance(id),
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
