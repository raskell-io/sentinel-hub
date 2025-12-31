"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Bell,
  Plus,
  RefreshCw,
  AlertTriangle,
  CheckCircle2,
  Clock,
  Trash2,
  Power,
  PowerOff,
} from "lucide-react";
import { formatDistanceToNow } from "date-fns";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { CreateAlertDialog } from "@/components/alerts/create-alert-dialog";
import { listAlerts, deleteAlert, updateAlert, Alert, ApiError } from "@/lib/api";
import { cn } from "@/lib/utils";

export default function AlertsPage() {
  const queryClient = useQueryClient();
  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [deleteAlertId, setDeleteAlertId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const { data, isLoading, refetch, isFetching } = useQuery({
    queryKey: ["alerts"],
    queryFn: listAlerts,
    refetchInterval: 30000,
  });

  const deleteMutation = useMutation({
    mutationFn: deleteAlert,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["alerts"] });
      setDeleteAlertId(null);
    },
    onError: (err) => {
      if (err instanceof ApiError) {
        setError(err.message);
      } else {
        setError("Failed to delete alert");
      }
    },
  });

  const toggleMutation = useMutation({
    mutationFn: ({ id, enabled }: { id: string; enabled: boolean }) =>
      updateAlert(id, { enabled }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["alerts"] });
    },
    onError: (err) => {
      if (err instanceof ApiError) {
        setError(err.message);
      } else {
        setError("Failed to update alert");
      }
    },
  });

  const alerts = data?.alerts ?? [];

  // Count by state
  const firingCount = alerts.filter((a) => a.state === "firing").length;
  const pendingCount = alerts.filter((a) => a.state === "pending").length;
  const okCount = alerts.filter((a) => a.state === "ok").length;

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Alerts</h1>
          <p className="text-muted-foreground">
            Configure alert rules for your fleet
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => refetch()}
            disabled={isFetching}
          >
            <RefreshCw
              className={`mr-2 h-4 w-4 ${isFetching ? "animate-spin" : ""}`}
            />
            Refresh
          </Button>
          <Button size="sm" onClick={() => setCreateDialogOpen(true)}>
            <Plus className="mr-2 h-4 w-4" />
            Create Alert
          </Button>
        </div>
      </div>

      {/* Error display */}
      {error && (
        <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
          {error}
          <Button
            variant="ghost"
            size="sm"
            className="ml-2 h-auto p-0 text-destructive hover:text-destructive"
            onClick={() => setError(null)}
          >
            Dismiss
          </Button>
        </div>
      )}

      {/* Stats */}
      <div className="grid gap-4 md:grid-cols-4">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Total Alerts</CardTitle>
            <Bell className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{alerts.length}</div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Firing</CardTitle>
            <AlertTriangle className="h-4 w-4 text-red-500" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-red-500">{firingCount}</div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Pending</CardTitle>
            <Clock className="h-4 w-4 text-yellow-500" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-yellow-500">
              {pendingCount}
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">OK</CardTitle>
            <CheckCircle2 className="h-4 w-4 text-green-500" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-green-500">{okCount}</div>
          </CardContent>
        </Card>
      </div>

      {/* Alerts Table */}
      <Card>
        <CardHeader>
          <CardTitle>Alert Rules</CardTitle>
          <CardDescription>
            Manage alert rules for monitoring your fleet
          </CardDescription>
        </CardHeader>
        <CardContent>
          {isLoading ? (
            <div className="space-y-2">
              {[...Array(5)].map((_, i) => (
                <Skeleton key={i} className="h-12 w-full" />
              ))}
            </div>
          ) : alerts.length === 0 ? (
            <div className="text-center py-12">
              <Bell className="mx-auto h-12 w-12 text-muted-foreground/50" />
              <h3 className="mt-4 text-lg font-semibold">No alerts configured</h3>
              <p className="mt-2 text-sm text-muted-foreground">
                Create your first alert rule to monitor your fleet.
              </p>
              <Button
                className="mt-4"
                onClick={() => setCreateDialogOpen(true)}
              >
                <Plus className="mr-2 h-4 w-4" />
                Create Alert
              </Button>
            </div>
          ) : (
            <div className="rounded-md border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Name</TableHead>
                    <TableHead>Condition</TableHead>
                    <TableHead>Severity</TableHead>
                    <TableHead>State</TableHead>
                    <TableHead>Last Triggered</TableHead>
                    <TableHead className="text-right">Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {alerts.map((alert) => (
                    <TableRow
                      key={alert.id}
                      className={cn(!alert.enabled && "opacity-50")}
                    >
                      <TableCell>
                        <div>
                          <p className="font-medium">{alert.name}</p>
                          {alert.description && (
                            <p className="text-xs text-muted-foreground">
                              {alert.description}
                            </p>
                          )}
                        </div>
                      </TableCell>
                      <TableCell className="font-mono text-sm">
                        {alert.condition} &gt; {alert.threshold}
                      </TableCell>
                      <TableCell>
                        <SeverityBadge severity={alert.severity} />
                      </TableCell>
                      <TableCell>
                        <StateBadge state={alert.state} enabled={alert.enabled} />
                      </TableCell>
                      <TableCell className="text-muted-foreground text-sm">
                        {alert.lastTriggeredAt
                          ? formatDistanceToNow(new Date(alert.lastTriggeredAt), {
                              addSuffix: true,
                            })
                          : "Never"}
                      </TableCell>
                      <TableCell className="text-right">
                        <div className="flex items-center justify-end gap-2">
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() =>
                              toggleMutation.mutate({
                                id: alert.id,
                                enabled: !alert.enabled,
                              })
                            }
                            disabled={toggleMutation.isPending}
                          >
                            {alert.enabled ? (
                              <PowerOff className="h-4 w-4" />
                            ) : (
                              <Power className="h-4 w-4" />
                            )}
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => setDeleteAlertId(alert.id)}
                          >
                            <Trash2 className="h-4 w-4 text-destructive" />
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </CardContent>
      </Card>

      {/* Create Dialog */}
      <CreateAlertDialog
        open={createDialogOpen}
        onOpenChange={setCreateDialogOpen}
      />

      {/* Delete Confirmation */}
      <AlertDialog
        open={!!deleteAlertId}
        onOpenChange={() => setDeleteAlertId(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete Alert</AlertDialogTitle>
            <AlertDialogDescription>
              Are you sure you want to delete this alert rule? This action cannot
              be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleteMutation.isPending}>
              Cancel
            </AlertDialogCancel>
            <AlertDialogAction
              onClick={() => deleteAlertId && deleteMutation.mutate(deleteAlertId)}
              disabled={deleteMutation.isPending}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {deleteMutation.isPending ? "Deleting..." : "Delete"}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

function SeverityBadge({ severity }: { severity: Alert["severity"] }) {
  const variants = {
    critical: "bg-red-500/10 text-red-700 border-red-500",
    warning: "bg-yellow-500/10 text-yellow-700 border-yellow-500",
    info: "bg-blue-500/10 text-blue-700 border-blue-500",
  };

  return (
    <Badge variant="outline" className={cn("capitalize", variants[severity])}>
      {severity}
    </Badge>
  );
}

function StateBadge({
  state,
  enabled,
}: {
  state: Alert["state"];
  enabled: boolean;
}) {
  if (!enabled) {
    return (
      <Badge variant="outline" className="text-muted-foreground">
        Disabled
      </Badge>
    );
  }

  const variants = {
    firing: "bg-red-500/10 text-red-700 border-red-500",
    pending: "bg-yellow-500/10 text-yellow-700 border-yellow-500",
    ok: "bg-green-500/10 text-green-700 border-green-500",
  };

  const icons = {
    firing: <AlertTriangle className="h-3 w-3 mr-1" />,
    pending: <Clock className="h-3 w-3 mr-1" />,
    ok: <CheckCircle2 className="h-3 w-3 mr-1" />,
  };

  return (
    <Badge variant="outline" className={cn("capitalize", variants[state])}>
      {icons[state]}
      {state}
    </Badge>
  );
}
