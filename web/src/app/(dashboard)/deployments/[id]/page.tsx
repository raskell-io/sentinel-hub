"use client";

import { use } from "react";
import Link from "next/link";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  ArrowLeft,
  Rocket,
  XCircle,
  CheckCircle2,
  Clock,
  Server,
  FileCode2,
  Loader2,
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
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { DeploymentStatusBadge } from "@/components/deployments/deployment-status-badge";
import {
  getDeployment,
  getConfig,
  listInstances,
  cancelDeployment,
  ApiError,
} from "@/lib/api";
import { useState } from "react";

interface PageProps {
  params: Promise<{ id: string }>;
}

export default function DeploymentDetailPage({ params }: PageProps) {
  const { id } = use(params);
  const queryClient = useQueryClient();
  const [cancelDialogOpen, setCancelDialogOpen] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const { data: deployment, isLoading } = useQuery({
    queryKey: ["deployment", id],
    queryFn: () => getDeployment(id),
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      // Poll every 2 seconds if in progress
      if (status === "pending" || status === "in-progress") {
        return 2000;
      }
      return false;
    },
  });

  const { data: config } = useQuery({
    queryKey: ["config", deployment?.configId],
    queryFn: () => getConfig(deployment!.configId),
    enabled: !!deployment?.configId,
  });

  const { data: instancesData } = useQuery({
    queryKey: ["instances"],
    queryFn: listInstances,
  });

  const cancelMutation = useMutation({
    mutationFn: () => cancelDeployment(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["deployment", id] });
      queryClient.invalidateQueries({ queryKey: ["deployments"] });
      setCancelDialogOpen(false);
    },
    onError: (err) => {
      if (err instanceof ApiError) {
        setError(err.message);
      } else {
        setError("Failed to cancel deployment");
      }
    },
  });

  const instances = instancesData?.instances ?? [];
  const targetInstanceMap = new Map(instances.map(i => [i.id, i]));

  const canCancel = deployment?.status === "pending" || deployment?.status === "in-progress";

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

  if (!deployment) {
    return (
      <div className="space-y-6">
        <Link href="/deployments">
          <Button variant="ghost" size="sm">
            <ArrowLeft className="mr-2 h-4 w-4" />
            Back to Deployments
          </Button>
        </Link>
        <Card>
          <CardContent className="py-12 text-center">
            <Rocket className="mx-auto h-12 w-12 text-muted-foreground/50" />
            <h3 className="mt-4 text-lg font-semibold">Deployment not found</h3>
            <p className="mt-2 text-sm text-muted-foreground">
              The requested deployment could not be found.
            </p>
          </CardContent>
        </Card>
      </div>
    );
  }

  const statusIcon = {
    pending: <Clock className="h-5 w-5 text-gray-500" />,
    "in-progress": <Loader2 className="h-5 w-5 text-blue-500 animate-spin" />,
    completed: <CheckCircle2 className="h-5 w-5 text-green-500" />,
    failed: <XCircle className="h-5 w-5 text-red-500" />,
    cancelled: <XCircle className="h-5 w-5 text-yellow-500" />,
  };

  return (
    <div className="space-y-6">
      {/* Back button */}
      <div>
        <Link href="/deployments">
          <Button variant="ghost" size="sm">
            <ArrowLeft className="mr-2 h-4 w-4" />
            Back to Deployments
          </Button>
        </Link>
      </div>

      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-4">
          <div className="rounded-lg bg-primary/10 p-3">
            {statusIcon[deployment.status]}
          </div>
          <div>
            <div className="flex items-center gap-2">
              <h1 className="text-3xl font-bold tracking-tight">
                Deployment
              </h1>
              <DeploymentStatusBadge status={deployment.status} />
            </div>
            <p className="text-muted-foreground font-mono text-sm">
              {deployment.id}
            </p>
          </div>
        </div>
        {canCancel && (
          <Button
            variant="destructive"
            size="sm"
            onClick={() => setCancelDialogOpen(true)}
          >
            <XCircle className="mr-2 h-4 w-4" />
            Cancel Deployment
          </Button>
        )}
      </div>

      {/* Error display */}
      {error && (
        <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
          {error}
        </div>
      )}

      {/* Details grid */}
      <div className="grid gap-6 md:grid-cols-2">
        {/* Configuration */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <FileCode2 className="h-4 w-4" />
              Configuration
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div>
              <p className="text-sm font-medium text-muted-foreground">Name</p>
              {config ? (
                <Link
                  href={`/configs/${config.id}`}
                  className="text-primary hover:underline"
                >
                  {config.name}
                </Link>
              ) : (
                <Skeleton className="h-5 w-32" />
              )}
            </div>
            <div>
              <p className="text-sm font-medium text-muted-foreground">Version</p>
              <p>v{deployment.configVersion}</p>
            </div>
            <div>
              <p className="text-sm font-medium text-muted-foreground">Strategy</p>
              <Badge variant="outline" className="capitalize mt-1">
                {deployment.strategy.replace("-", " ")}
              </Badge>
            </div>
          </CardContent>
        </Card>

        {/* Timing */}
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Clock className="h-4 w-4" />
              Timing
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div>
              <p className="text-sm font-medium text-muted-foreground">Created</p>
              <p>{format(new Date(deployment.createdAt), "PPpp")}</p>
            </div>
            {deployment.startedAt && (
              <div>
                <p className="text-sm font-medium text-muted-foreground">Started</p>
                <p>{format(new Date(deployment.startedAt), "PPpp")}</p>
              </div>
            )}
            {deployment.completedAt && (
              <div>
                <p className="text-sm font-medium text-muted-foreground">Completed</p>
                <p>{format(new Date(deployment.completedAt), "PPpp")}</p>
              </div>
            )}
            {deployment.startedAt && deployment.completedAt && (
              <div>
                <p className="text-sm font-medium text-muted-foreground">Duration</p>
                <p>
                  {formatDistanceToNow(new Date(deployment.startedAt), {
                    includeSeconds: true,
                  }).replace("about ", "")}
                </p>
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      {/* Target Instances */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Server className="h-4 w-4" />
            Target Instances
          </CardTitle>
          <CardDescription>
            {deployment.targetInstances.length} instance(s) targeted for this deployment
          </CardDescription>
        </CardHeader>
        <CardContent>
          {deployment.targetInstances.length === 0 ? (
            <p className="text-muted-foreground text-sm">No instances targeted</p>
          ) : (
            <div className="grid gap-2 md:grid-cols-2 lg:grid-cols-3">
              {deployment.targetInstances.map((instanceId) => {
                const instance = targetInstanceMap.get(instanceId);
                return (
                  <div
                    key={instanceId}
                    className="flex items-center gap-3 rounded-lg border p-3"
                  >
                    <Server className="h-4 w-4 text-muted-foreground" />
                    <div className="flex-1 min-w-0">
                      {instance ? (
                        <>
                          <p className="font-medium truncate">{instance.name}</p>
                          <p className="text-xs text-muted-foreground truncate">
                            {instance.hostname}
                          </p>
                        </>
                      ) : (
                        <p className="font-mono text-sm truncate">{instanceId}</p>
                      )}
                    </div>
                    {deployment.status === "completed" && (
                      <CheckCircle2 className="h-4 w-4 text-green-500" />
                    )}
                    {deployment.status === "in-progress" && (
                      <Loader2 className="h-4 w-4 text-blue-500 animate-spin" />
                    )}
                  </div>
                );
              })}
            </div>
          )}
        </CardContent>
      </Card>

      {/* Cancel Dialog */}
      <Dialog open={cancelDialogOpen} onOpenChange={setCancelDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Cancel Deployment</DialogTitle>
            <DialogDescription>
              Are you sure you want to cancel this deployment? Instances that have
              already received the configuration will not be rolled back.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setCancelDialogOpen(false)}
              disabled={cancelMutation.isPending}
            >
              Keep Running
            </Button>
            <Button
              variant="destructive"
              onClick={() => cancelMutation.mutate()}
              disabled={cancelMutation.isPending}
            >
              {cancelMutation.isPending ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Cancelling...
                </>
              ) : (
                "Cancel Deployment"
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
