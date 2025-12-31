"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { Loader2, Rocket } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { listConfigs, listInstances, createDeployment, ApiError } from "@/lib/api";

const deploymentSchema = z.object({
  configId: z.string().min(1, "Please select a configuration"),
  strategy: z.enum(["all-at-once", "rolling", "canary"]),
  targetType: z.enum(["all", "selected"]),
});

type DeploymentFormValues = z.infer<typeof deploymentSchema>;

interface CreateDeploymentDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  preselectedConfigId?: string;
}

export function CreateDeploymentDialog({
  open,
  onOpenChange,
  preselectedConfigId,
}: CreateDeploymentDialogProps) {
  const router = useRouter();
  const queryClient = useQueryClient();
  const [error, setError] = useState<string | null>(null);

  const { data: configsData } = useQuery({
    queryKey: ["configs"],
    queryFn: listConfigs,
    enabled: open,
  });

  const { data: instancesData } = useQuery({
    queryKey: ["instances"],
    queryFn: listInstances,
    enabled: open,
  });

  const form = useForm<DeploymentFormValues>({
    resolver: zodResolver(deploymentSchema),
    defaultValues: {
      configId: preselectedConfigId || "",
      strategy: "all-at-once",
      targetType: "all",
    },
  });

  const mutation = useMutation({
    mutationFn: (data: DeploymentFormValues) => {
      const instances = instancesData?.instances || [];
      return createDeployment({
        configId: data.configId,
        strategy: data.strategy,
        targetInstances: data.targetType === "all" ? instances.map(i => i.id) : [],
      });
    },
    onSuccess: (deployment) => {
      queryClient.invalidateQueries({ queryKey: ["deployments"] });
      onOpenChange(false);
      form.reset();
      router.push(`/deployments/${deployment.id}`);
    },
    onError: (err) => {
      if (err instanceof ApiError) {
        setError(err.message);
      } else {
        setError("An unexpected error occurred");
      }
    },
  });

  function onSubmit(data: DeploymentFormValues) {
    setError(null);
    mutation.mutate(data);
  }

  function handleOpenChange(open: boolean) {
    if (!open) {
      form.reset();
      setError(null);
    }
    onOpenChange(open);
  }

  const configs = configsData?.configs || [];
  const instances = instancesData?.instances || [];
  const onlineInstances = instances.filter(i => i.status === "online");

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Rocket className="h-5 w-5" />
            Create Deployment
          </DialogTitle>
          <DialogDescription>
            Deploy a configuration to your Sentinel instances.
          </DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
            {error && (
              <div className="rounded-md bg-destructive/10 p-3 text-sm text-destructive">
                {error}
              </div>
            )}

            <FormField
              control={form.control}
              name="configId"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Configuration</FormLabel>
                  <Select
                    onValueChange={field.onChange}
                    defaultValue={field.value}
                    disabled={mutation.isPending}
                  >
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue placeholder="Select a configuration" />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent>
                      {configs.map((config) => (
                        <SelectItem key={config.id} value={config.id}>
                          {config.name} (v{config.currentVersion})
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name="strategy"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Deployment Strategy</FormLabel>
                  <Select
                    onValueChange={field.onChange}
                    defaultValue={field.value}
                    disabled={mutation.isPending}
                  >
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent>
                      <SelectItem value="all-at-once">
                        All at once
                      </SelectItem>
                      <SelectItem value="rolling">
                        Rolling
                      </SelectItem>
                      <SelectItem value="canary">
                        Canary
                      </SelectItem>
                    </SelectContent>
                  </Select>
                  <FormDescription>
                    {field.value === "all-at-once" && "Deploy to all instances simultaneously"}
                    {field.value === "rolling" && "Deploy in batches with health checks"}
                    {field.value === "canary" && "Deploy to one instance first, then the rest"}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <div className="rounded-md bg-muted p-3 text-sm">
              <p className="font-medium">Target Instances</p>
              <p className="text-muted-foreground mt-1">
                {onlineInstances.length} of {instances.length} instances online
              </p>
            </div>

            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => handleOpenChange(false)}
                disabled={mutation.isPending}
              >
                Cancel
              </Button>
              <Button
                type="submit"
                disabled={mutation.isPending || configs.length === 0}
              >
                {mutation.isPending ? (
                  <>
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                    Deploying...
                  </>
                ) : (
                  <>
                    <Rocket className="mr-2 h-4 w-4" />
                    Deploy
                  </>
                )}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
