import { useAuthStore, User } from "@/stores/auth-store";

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

function getAuthHeaders(): HeadersInit {
  const token = useAuthStore.getState().accessToken;
  const headers: HeadersInit = {
    "Content-Type": "application/json",
  };
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }
  return headers;
}

async function handleResponse<T>(response: Response): Promise<T> {
  if (!response.ok) {
    // On 401, clear auth state
    if (response.status === 401) {
      useAuthStore.getState().clearAuth();
    }
    const error = await response.json().catch(() => ({}));
    throw new ApiError(
      response.status,
      error.error?.code || "UNKNOWN_ERROR",
      error.error?.message || response.statusText
    );
  }
  return response.json();
}

// Auth
export interface LoginCredentials {
  email: string;
  password: string;
}

export interface LoginResponse {
  access_token: string;
  refresh_token: string;
  token_type: string;
  expires_in: number;
  user: User;
}

export interface RefreshResponse {
  access_token: string;
  token_type: string;
  expires_in: number;
}

export async function login(credentials: LoginCredentials): Promise<LoginResponse> {
  const response = await fetch(`${API_BASE}/auth/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(credentials),
  });
  return handleResponse<LoginResponse>(response);
}

export async function logout(): Promise<void> {
  const response = await fetch(`${API_BASE}/auth/logout`, {
    method: "POST",
    headers: getAuthHeaders(),
  });
  if (!response.ok && response.status !== 401) {
    const error = await response.json().catch(() => ({}));
    throw new ApiError(
      response.status,
      error.error?.code || "UNKNOWN_ERROR",
      error.error?.message || response.statusText
    );
  }
}

export async function refreshToken(refreshToken: string): Promise<RefreshResponse> {
  const response = await fetch(`${API_BASE}/auth/refresh`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ refresh_token: refreshToken }),
  });
  return handleResponse<RefreshResponse>(response);
}

export async function getCurrentUser(): Promise<User> {
  const response = await fetch(`${API_BASE}/auth/me`, {
    headers: getAuthHeaders(),
  });
  return handleResponse<User>(response);
}

// Users (admin only)
export interface CreateUserInput {
  email: string;
  password: string;
  name: string;
  role: "admin" | "operator" | "viewer";
}

export interface UpdateUserInput {
  name?: string;
  role?: "admin" | "operator" | "viewer";
  password?: string;
}

export async function listUsers() {
  const response = await fetch(`${API_BASE}/users`, {
    headers: getAuthHeaders(),
  });
  return handleResponse<{ users: User[] }>(response);
}

export async function getUser(id: string) {
  const response = await fetch(`${API_BASE}/users/${id}`, {
    headers: getAuthHeaders(),
  });
  return handleResponse<User>(response);
}

export async function createUser(data: CreateUserInput) {
  const response = await fetch(`${API_BASE}/users`, {
    method: "POST",
    headers: getAuthHeaders(),
    body: JSON.stringify(data),
  });
  return handleResponse<User>(response);
}

export async function updateUser(id: string, data: UpdateUserInput) {
  const response = await fetch(`${API_BASE}/users/${id}`, {
    method: "PUT",
    headers: getAuthHeaders(),
    body: JSON.stringify(data),
  });
  return handleResponse<User>(response);
}

export async function deleteUser(id: string) {
  const response = await fetch(`${API_BASE}/users/${id}`, {
    method: "DELETE",
    headers: getAuthHeaders(),
  });
  if (!response.ok) {
    const error = await response.json().catch(() => ({}));
    throw new ApiError(
      response.status,
      error.error?.code || "UNKNOWN_ERROR",
      error.error?.message || response.statusText
    );
  }
}

// Pagination types
export interface PaginationParams {
  page?: number;
  limit?: number;
}

export interface PaginatedResponse<T> {
  items: T[];
  total: number;
  page: number;
  limit: number;
  totalPages: number;
}

// Instances
export async function listInstances(params?: PaginationParams) {
  const searchParams = new URLSearchParams();
  if (params?.page) searchParams.set("page", params.page.toString());
  if (params?.limit) searchParams.set("limit", params.limit.toString());

  const response = await fetch(`${API_BASE}/instances?${searchParams}`, {
    headers: getAuthHeaders(),
  });
  return handleResponse<{ instances: Instance[]; total?: number }>(response);
}

export async function getInstance(id: string) {
  const response = await fetch(`${API_BASE}/instances/${id}`, {
    headers: getAuthHeaders(),
  });
  return handleResponse<Instance>(response);
}

// Configurations
export async function listConfigs(params?: PaginationParams) {
  const searchParams = new URLSearchParams();
  if (params?.page) searchParams.set("page", params.page.toString());
  if (params?.limit) searchParams.set("limit", params.limit.toString());

  const response = await fetch(`${API_BASE}/configs?${searchParams}`, {
    headers: getAuthHeaders(),
  });
  return handleResponse<{ configs: Config[]; total?: number }>(response);
}

export async function getConfig(id: string) {
  const response = await fetch(`${API_BASE}/configs/${id}`, {
    headers: getAuthHeaders(),
  });
  return handleResponse<Config>(response);
}

export async function createConfig(data: CreateConfigInput) {
  const response = await fetch(`${API_BASE}/configs`, {
    method: "POST",
    headers: getAuthHeaders(),
    body: JSON.stringify(data),
  });
  return handleResponse<Config>(response);
}

export async function updateConfig(id: string, data: UpdateConfigInput) {
  const response = await fetch(`${API_BASE}/configs/${id}`, {
    method: "PUT",
    headers: getAuthHeaders(),
    body: JSON.stringify(data),
  });
  return handleResponse<Config>(response);
}

export async function deleteConfig(id: string) {
  const response = await fetch(`${API_BASE}/configs/${id}`, {
    method: "DELETE",
    headers: getAuthHeaders(),
  });
  if (!response.ok) {
    const error = await response.json().catch(() => ({}));
    throw new ApiError(
      response.status,
      error.error?.code || "UNKNOWN_ERROR",
      error.error?.message || response.statusText
    );
  }
}

export async function getConfigVersions(configId: string) {
  const response = await fetch(`${API_BASE}/configs/${configId}/versions`, {
    headers: getAuthHeaders(),
  });
  return handleResponse<{ versions: ConfigVersion[] }>(response);
}

export async function getConfigVersion(configId: string, version: number) {
  const response = await fetch(`${API_BASE}/configs/${configId}/versions/${version}`, {
    headers: getAuthHeaders(),
  });
  return handleResponse<ConfigVersion>(response);
}

export async function rollbackConfig(configId: string, version: number) {
  const response = await fetch(`${API_BASE}/configs/${configId}/rollback`, {
    method: "POST",
    headers: getAuthHeaders(),
    body: JSON.stringify({ version }),
  });
  return handleResponse<Config>(response);
}

export async function validateConfig(content: string) {
  const response = await fetch(`${API_BASE}/configs/validate`, {
    method: "POST",
    headers: getAuthHeaders(),
    body: JSON.stringify({ content }),
  });
  return handleResponse<{ valid: boolean; errors?: string[] }>(response);
}

// Deployments
export async function listDeployments(params?: PaginationParams) {
  const searchParams = new URLSearchParams();
  if (params?.page) searchParams.set("page", params.page.toString());
  if (params?.limit) searchParams.set("limit", params.limit.toString());

  const response = await fetch(`${API_BASE}/deployments?${searchParams}`, {
    headers: getAuthHeaders(),
  });
  return handleResponse<{ deployments: Deployment[]; total?: number }>(response);
}

export async function createDeployment(data: CreateDeploymentInput) {
  const response = await fetch(`${API_BASE}/deployments`, {
    method: "POST",
    headers: getAuthHeaders(),
    body: JSON.stringify(data),
  });
  return handleResponse<Deployment>(response);
}

export async function getDeployment(id: string) {
  const response = await fetch(`${API_BASE}/deployments/${id}`, {
    headers: getAuthHeaders(),
  });
  return handleResponse<Deployment>(response);
}

export async function cancelDeployment(id: string) {
  const response = await fetch(`${API_BASE}/deployments/${id}/cancel`, {
    method: "POST",
    headers: getAuthHeaders(),
  });
  return handleResponse<Deployment>(response);
}

// Audit Logs (admin only)
export interface AuditLog {
  id: string;
  userId: string;
  userEmail: string;
  action: string;
  resourceType: string;
  resourceId: string;
  details: Record<string, unknown>;
  ipAddress: string;
  createdAt: string;
}

export async function listAuditLogs(params?: {
  limit?: number;
  offset?: number;
  userId?: string;
  action?: string;
  resourceType?: string;
}) {
  const searchParams = new URLSearchParams();
  if (params?.limit) searchParams.set("limit", params.limit.toString());
  if (params?.offset) searchParams.set("offset", params.offset.toString());
  if (params?.userId) searchParams.set("user_id", params.userId);
  if (params?.action) searchParams.set("action", params.action);
  if (params?.resourceType) searchParams.set("resource_type", params.resourceType);

  const response = await fetch(`${API_BASE}/audit-logs?${searchParams}`, {
    headers: getAuthHeaders(),
  });
  return handleResponse<{ logs: AuditLog[]; total: number }>(response);
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

// Metrics
export interface MetricsSummary {
  totalRequests: number;
  totalErrors: number;
  errorRate: number;
  avgLatencyMs: number;
  p50LatencyMs: number;
  p95LatencyMs: number;
  p99LatencyMs: number;
  requestsPerSecond: number;
  activeConnections: number;
  bytesIn: number;
  bytesOut: number;
  periodStart: string;
  periodEnd: string;
}

export interface TimeSeriesPoint {
  timestamp: string;
  value: number;
}

export interface TimeSeriesData {
  metric: string;
  points: TimeSeriesPoint[];
}

export interface FleetMetrics {
  summary: MetricsSummary;
  timeSeries: {
    requests: TimeSeriesPoint[];
    errors: TimeSeriesPoint[];
    latencyP50: TimeSeriesPoint[];
    latencyP95: TimeSeriesPoint[];
    latencyP99: TimeSeriesPoint[];
  };
  byInstance: Record<string, MetricsSummary>;
}

export interface InstanceMetrics {
  instanceId: string;
  summary: MetricsSummary;
  timeSeries: {
    requests: TimeSeriesPoint[];
    errors: TimeSeriesPoint[];
    latencyP50: TimeSeriesPoint[];
    latencyP95: TimeSeriesPoint[];
    latencyP99: TimeSeriesPoint[];
  };
  upstreams: UpstreamMetrics[];
}

export interface UpstreamMetrics {
  name: string;
  healthy: boolean;
  totalRequests: number;
  totalErrors: number;
  avgLatencyMs: number;
  activeConnections: number;
}

export async function getFleetMetrics(params?: {
  period?: "1h" | "6h" | "24h" | "7d";
}): Promise<FleetMetrics> {
  const searchParams = new URLSearchParams();
  if (params?.period) searchParams.set("period", params.period);

  const response = await fetch(`${API_BASE}/metrics/fleet?${searchParams}`, {
    headers: getAuthHeaders(),
  });
  return handleResponse<FleetMetrics>(response);
}

export async function getInstanceMetrics(
  instanceId: string,
  params?: { period?: "1h" | "6h" | "24h" | "7d" }
): Promise<InstanceMetrics> {
  const searchParams = new URLSearchParams();
  if (params?.period) searchParams.set("period", params.period);

  const response = await fetch(
    `${API_BASE}/instances/${instanceId}/metrics?${searchParams}`,
    { headers: getAuthHeaders() }
  );
  return handleResponse<InstanceMetrics>(response);
}

// Alerts
export interface Alert {
  id: string;
  name: string;
  description?: string;
  severity: "critical" | "warning" | "info";
  condition: string;
  threshold: number;
  enabled: boolean;
  state: "ok" | "firing" | "pending";
  lastTriggeredAt?: string;
  createdAt: string;
  updatedAt: string;
}

export interface AlertRule {
  name: string;
  description?: string;
  severity: "critical" | "warning" | "info";
  metric: string;
  operator: "gt" | "lt" | "gte" | "lte" | "eq";
  threshold: number;
  duration: string;
  enabled: boolean;
  notificationChannels?: string[];
}

export async function listAlerts(): Promise<{ alerts: Alert[] }> {
  const response = await fetch(`${API_BASE}/alerts`, {
    headers: getAuthHeaders(),
  });
  return handleResponse<{ alerts: Alert[] }>(response);
}

export async function createAlert(data: AlertRule): Promise<Alert> {
  const response = await fetch(`${API_BASE}/alerts`, {
    method: "POST",
    headers: getAuthHeaders(),
    body: JSON.stringify(data),
  });
  return handleResponse<Alert>(response);
}

export async function updateAlert(id: string, data: Partial<AlertRule>): Promise<Alert> {
  const response = await fetch(`${API_BASE}/alerts/${id}`, {
    method: "PUT",
    headers: getAuthHeaders(),
    body: JSON.stringify(data),
  });
  return handleResponse<Alert>(response);
}

export async function deleteAlert(id: string): Promise<void> {
  const response = await fetch(`${API_BASE}/alerts/${id}`, {
    method: "DELETE",
    headers: getAuthHeaders(),
  });
  if (!response.ok) {
    const error = await response.json().catch(() => ({}));
    throw new ApiError(
      response.status,
      error.error?.code || "UNKNOWN_ERROR",
      error.error?.message || response.statusText
    );
  }
}
