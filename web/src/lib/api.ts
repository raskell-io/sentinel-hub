const API_BASE = "/api/v1";

export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string
  ) {
    super(message);
    this.name = "ApiError";
  }
}

async function handleResponse<T>(response: Response): Promise<T> {
  if (!response.ok) {
    const error = await response.json().catch(() => ({}));
    throw new ApiError(
      response.status,
      error.error?.code || "UNKNOWN_ERROR",
      error.error?.message || response.statusText
    );
  }
  return response.json();
}

// Instances
export async function listInstances() {
  const response = await fetch(`${API_BASE}/instances`);
  return handleResponse<{ instances: Instance[] }>(response);
}

export async function getInstance(id: string) {
  const response = await fetch(`${API_BASE}/instances/${id}`);
  return handleResponse<Instance>(response);
}

// Configurations
export async function listConfigs() {
  const response = await fetch(`${API_BASE}/configs`);
  return handleResponse<{ configs: Config[] }>(response);
}

export async function getConfig(id: string) {
  const response = await fetch(`${API_BASE}/configs/${id}`);
  return handleResponse<Config>(response);
}

export async function createConfig(data: CreateConfigInput) {
  const response = await fetch(`${API_BASE}/configs`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
  return handleResponse<Config>(response);
}

export async function updateConfig(id: string, data: UpdateConfigInput) {
  const response = await fetch(`${API_BASE}/configs/${id}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
  return handleResponse<Config>(response);
}

// Deployments
export async function listDeployments() {
  const response = await fetch(`${API_BASE}/deployments`);
  return handleResponse<{ deployments: Deployment[] }>(response);
}

export async function createDeployment(data: CreateDeploymentInput) {
  const response = await fetch(`${API_BASE}/deployments`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
  return handleResponse<Deployment>(response);
}

// Types
export interface Instance {
  id: string;
  name: string;
  hostname: string;
  status: "online" | "offline" | "unhealthy";
  agentVersion: string;
  sentinelVersion: string;
  currentConfigId?: string;
  currentConfigVersion?: number;
  labels: Record<string, string>;
  lastSeenAt: string;
  createdAt: string;
  updatedAt: string;
}

export interface Config {
  id: string;
  name: string;
  description?: string;
  content: string;
  contentHash: string;
  currentVersion: number;
  createdBy: string;
  createdAt: string;
  updatedAt: string;
}

export interface ConfigVersion {
  id: string;
  configId: string;
  version: number;
  content: string;
  contentHash: string;
  changeSummary?: string;
  createdBy: string;
  createdAt: string;
}

export interface Deployment {
  id: string;
  configId: string;
  configVersion: number;
  targetInstances: string[];
  strategy: "all-at-once" | "rolling" | "canary";
  status: "pending" | "in-progress" | "completed" | "failed" | "cancelled";
  startedAt?: string;
  completedAt?: string;
  createdBy: string;
  createdAt: string;
}

export interface CreateConfigInput {
  name: string;
  description?: string;
  content: string;
}

export interface UpdateConfigInput {
  description?: string;
  content: string;
  changeSummary?: string;
}

export interface CreateDeploymentInput {
  configId: string;
  configVersion?: number;
  targetInstances?: string[];
  targetLabels?: Record<string, string>;
  strategy?: "all-at-once" | "rolling" | "canary";
}
