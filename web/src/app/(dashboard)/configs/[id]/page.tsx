"use client";

import { use, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  ArrowLeft,
  FileCode2,
  Save,
  History,
  RotateCcw,
  Loader2,
  Clock,
  GitBranch,
} from "lucide-react";
import { formatDistanceToNow, format } from "date-fns";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { ConfigEditor } from "@/components/configs/config-editor";
import {
  getConfig,
  updateConfig,
  getConfigVersions,
  rollbackConfig,
  ApiError,
  ConfigVersion,
} from "@/lib/api";

interface PageProps {
  params: Promise<{ id: string }>;
}

export default function ConfigDetailPage({ params }: PageProps) {
  const { id } = use(params);
  const router = useRouter();
  const queryClient = useQueryClient();

  const [content, setContent] = useState<string>("");
  const [hasChanges, setHasChanges] = useState(false);
  const [changeSummary, setChangeSummary] = useState("");
  const [saveDialogOpen, setSaveDialogOpen] = useState(false);
  const [historyDialogOpen, setHistoryDialogOpen] = useState(false);
  const [rollbackDialogOpen, setRollbackDialogOpen] = useState(false);
  const [selectedVersion, setSelectedVersion] = useState<ConfigVersion | null>(null);
  const [error, setError] = useState<string | null>(null);

  const { data: config, isLoading } = useQuery({
    queryKey: ["config", id],
    queryFn: () => getConfig(id),
  });

  const { data: versionsData } = useQuery({
    queryKey: ["config-versions", id],
    queryFn: () => getConfigVersions(id),
    enabled: historyDialogOpen,
  });

  // Initialize content when config loads
  useState(() => {
    if (config && !hasChanges) {
      setContent(config.content);
    }
  });

  // Update content when config changes (initial load)
  if (config && content === "" && !hasChanges) {
    setContent(config.content);
  }

  const updateMutation = useMutation({
    mutationFn: () =>
      updateConfig(id, {
        content,
        changeSummary: changeSummary || undefined,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["config", id] });
      queryClient.invalidateQueries({ queryKey: ["configs"] });
      setHasChanges(false);
      setChangeSummary("");
      setSaveDialogOpen(false);
      setError(null);
    },
    onError: (err) => {
      if (err instanceof ApiError) {
        setError(err.message);
      } else {
        setError("Failed to save configuration");
      }
    },
  });

  const rollbackMutation = useMutation({
    mutationFn: (version: number) => rollbackConfig(id, version),
    onSuccess: (newConfig) => {
      queryClient.invalidateQueries({ queryKey: ["config", id] });
      queryClient.invalidateQueries({ queryKey: ["configs"] });
      setContent(newConfig.content);
      setHasChanges(false);
      setRollbackDialogOpen(false);
      setSelectedVersion(null);
    },
    onError: (err) => {
      if (err instanceof ApiError) {
        setError(err.message);
      } else {
        setError("Failed to rollback configuration");
      }
    },
  });

  function handleContentChange(newContent: string) {
    setContent(newContent);
    setHasChanges(newContent !== config?.content);
  }

  function handleSave() {
    setSaveDialogOpen(true);
  }

  function handleConfirmSave() {
    updateMutation.mutate();
  }

  function handleRollbackClick(version: ConfigVersion) {
    setSelectedVersion(version);
    setHistoryDialogOpen(false);
    setRollbackDialogOpen(true);
  }

  function handleConfirmRollback() {
    if (selectedVersion) {
      rollbackMutation.mutate(selectedVersion.version);
    }
  }

  if (isLoading) {
    return (
      <div className="space-y-6">
        <Skeleton className="h-8 w-48" />
        <Skeleton className="h-[500px]" />
      </div>
    );
  }

  if (!config) {
    return (
      <div className="space-y-6">
        <Link href="/configs">
          <Button variant="ghost" size="sm">
            <ArrowLeft className="mr-2 h-4 w-4" />
            Back to Configurations
          </Button>
        </Link>
        <Card>
          <CardContent className="py-12 text-center">
            <FileCode2 className="mx-auto h-12 w-12 text-muted-foreground/50" />
            <h3 className="mt-4 text-lg font-semibold">Configuration not found</h3>
            <p className="mt-2 text-sm text-muted-foreground">
              The requested configuration could not be found.
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
        <Link href="/configs">
          <Button variant="ghost" size="sm">
            <ArrowLeft className="mr-2 h-4 w-4" />
            Back to Configurations
          </Button>
        </Link>
      </div>

      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-4">
          <div className="rounded-lg bg-primary/10 p-3">
            <FileCode2 className="h-6 w-6 text-primary" />
          </div>
          <div>
            <div className="flex items-center gap-2">
              <h1 className="text-3xl font-bold tracking-tight">{config.name}</h1>
              <Badge variant="secondary">v{config.currentVersion}</Badge>
              {hasChanges && (
                <Badge variant="outline" className="border-yellow-500 text-yellow-600">
                  Unsaved changes
                </Badge>
              )}
            </div>
            {config.description && (
              <p className="text-muted-foreground">{config.description}</p>
            )}
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => setHistoryDialogOpen(true)}
          >
            <History className="mr-2 h-4 w-4" />
            History
          </Button>
          <Button
            size="sm"
            onClick={handleSave}
            disabled={!hasChanges || updateMutation.isPending}
          >
            {updateMutation.isPending ? (
              <>
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                Saving...
              </>
            ) : (
              <>
                <Save className="mr-2 h-4 w-4" />
                Save
              </>
            )}
          </Button>
        </div>
      </div>

      {/* Error display */}
      {error && (
        <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
          {error}
        </div>
      )}

      {/* Editor */}
      <Card>
        <CardHeader>
          <CardTitle>Configuration Editor</CardTitle>
          <CardDescription>
            Edit the KDL configuration below. Changes are saved as a new version.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <ConfigEditor
            value={content}
            onChange={handleContentChange}
            height="500px"
          />
        </CardContent>
      </Card>

      {/* Metadata */}
      <div className="grid gap-4 md:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="text-base flex items-center gap-2">
              <Clock className="h-4 w-4" />
              Last Updated
            </CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-2xl font-semibold">
              {formatDistanceToNow(new Date(config.updatedAt), { addSuffix: true })}
            </p>
            <p className="text-sm text-muted-foreground">
              {format(new Date(config.updatedAt), "PPpp")}
            </p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle className="text-base flex items-center gap-2">
              <GitBranch className="h-4 w-4" />
              Version Info
            </CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-2xl font-semibold">Version {config.currentVersion}</p>
            <p className="text-sm text-muted-foreground font-mono">
              Hash: {config.contentHash.slice(0, 12)}...
            </p>
          </CardContent>
        </Card>
      </div>

      {/* Save Dialog */}
      <Dialog open={saveDialogOpen} onOpenChange={setSaveDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Save Configuration</DialogTitle>
            <DialogDescription>
              This will create a new version of the configuration.
            </DialogDescription>
          </DialogHeader>
          <div className="py-4">
            <label className="text-sm font-medium">Change Summary (optional)</label>
            <Input
              className="mt-2"
              placeholder="Brief description of changes..."
              value={changeSummary}
              onChange={(e) => setChangeSummary(e.target.value)}
            />
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setSaveDialogOpen(false)}
              disabled={updateMutation.isPending}
            >
              Cancel
            </Button>
            <Button onClick={handleConfirmSave} disabled={updateMutation.isPending}>
              {updateMutation.isPending ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Saving...
                </>
              ) : (
                "Save Changes"
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* History Dialog */}
      <Dialog open={historyDialogOpen} onOpenChange={setHistoryDialogOpen}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>Version History</DialogTitle>
            <DialogDescription>
              View previous versions and rollback if needed.
            </DialogDescription>
          </DialogHeader>
          <div className="max-h-[400px] overflow-auto">
            {versionsData?.versions && versionsData.versions.length > 0 ? (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Version</TableHead>
                    <TableHead>Change Summary</TableHead>
                    <TableHead>Created</TableHead>
                    <TableHead className="w-[100px]"></TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {versionsData.versions.map((version) => (
                    <TableRow key={version.id}>
                      <TableCell>
                        <Badge variant="outline">v{version.version}</Badge>
                      </TableCell>
                      <TableCell className="text-muted-foreground">
                        {version.changeSummary || "—"}
                      </TableCell>
                      <TableCell className="text-sm text-muted-foreground">
                        {formatDistanceToNow(new Date(version.createdAt), {
                          addSuffix: true,
                        })}
                      </TableCell>
                      <TableCell>
                        {version.version !== config.currentVersion && (
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => handleRollbackClick(version)}
                          >
                            <RotateCcw className="mr-1 h-3 w-3" />
                            Rollback
                          </Button>
                        )}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            ) : (
              <div className="py-8 text-center text-muted-foreground">
                <History className="mx-auto h-8 w-8 mb-2 opacity-50" />
                <p>No version history available</p>
              </div>
            )}
          </div>
        </DialogContent>
      </Dialog>

      {/* Rollback Dialog */}
      <Dialog open={rollbackDialogOpen} onOpenChange={setRollbackDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Rollback Configuration</DialogTitle>
            <DialogDescription>
              Are you sure you want to rollback to version {selectedVersion?.version}?
              This will create a new version with the old content.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => setRollbackDialogOpen(false)}
              disabled={rollbackMutation.isPending}
            >
              Cancel
            </Button>
            <Button
              onClick={handleConfirmRollback}
              disabled={rollbackMutation.isPending}
            >
              {rollbackMutation.isPending ? (
                <>
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                  Rolling back...
                </>
              ) : (
                <>
                  <RotateCcw className="mr-2 h-4 w-4" />
                  Rollback
                </>
              )}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
