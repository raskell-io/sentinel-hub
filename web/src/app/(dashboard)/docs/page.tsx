"use client";

import { Book, ExternalLink, Code, Server, FileCode2, Rocket, Bell, Users, ScrollText } from "lucide-react";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";

const apiEndpoints = [
  {
    category: "Authentication",
    icon: Users,
    endpoints: [
      { method: "POST", path: "/api/v1/auth/login", description: "Authenticate user and get tokens" },
      { method: "POST", path: "/api/v1/auth/logout", description: "Invalidate current session" },
      { method: "POST", path: "/api/v1/auth/refresh", description: "Refresh access token" },
      { method: "GET", path: "/api/v1/auth/me", description: "Get current user info" },
    ],
  },
  {
    category: "Instances",
    icon: Server,
    endpoints: [
      { method: "GET", path: "/api/v1/instances", description: "List all Sentinel instances" },
      { method: "GET", path: "/api/v1/instances/:id", description: "Get instance details" },
      { method: "GET", path: "/api/v1/instances/:id/metrics", description: "Get instance metrics" },
    ],
  },
  {
    category: "Configurations",
    icon: FileCode2,
    endpoints: [
      { method: "GET", path: "/api/v1/configs", description: "List all configurations" },
      { method: "POST", path: "/api/v1/configs", description: "Create new configuration" },
      { method: "GET", path: "/api/v1/configs/:id", description: "Get configuration details" },
      { method: "PUT", path: "/api/v1/configs/:id", description: "Update configuration" },
      { method: "DELETE", path: "/api/v1/configs/:id", description: "Delete configuration" },
      { method: "GET", path: "/api/v1/configs/:id/versions", description: "List config versions" },
      { method: "POST", path: "/api/v1/configs/:id/rollback", description: "Rollback to version" },
      { method: "POST", path: "/api/v1/configs/validate", description: "Validate KDL config" },
    ],
  },
  {
    category: "Deployments",
    icon: Rocket,
    endpoints: [
      { method: "GET", path: "/api/v1/deployments", description: "List all deployments" },
      { method: "POST", path: "/api/v1/deployments", description: "Create new deployment" },
      { method: "GET", path: "/api/v1/deployments/:id", description: "Get deployment status" },
      { method: "POST", path: "/api/v1/deployments/:id/cancel", description: "Cancel deployment" },
    ],
  },
  {
    category: "Metrics",
    icon: Bell,
    endpoints: [
      { method: "GET", path: "/api/v1/metrics/fleet", description: "Get fleet-wide metrics" },
    ],
  },
  {
    category: "Alerts",
    icon: Bell,
    endpoints: [
      { method: "GET", path: "/api/v1/alerts", description: "List alert rules" },
      { method: "POST", path: "/api/v1/alerts", description: "Create alert rule" },
      { method: "PUT", path: "/api/v1/alerts/:id", description: "Update alert rule" },
      { method: "DELETE", path: "/api/v1/alerts/:id", description: "Delete alert rule" },
    ],
  },
  {
    category: "Users (Admin)",
    icon: Users,
    endpoints: [
      { method: "GET", path: "/api/v1/users", description: "List all users" },
      { method: "POST", path: "/api/v1/users", description: "Create new user" },
      { method: "GET", path: "/api/v1/users/:id", description: "Get user details" },
      { method: "PUT", path: "/api/v1/users/:id", description: "Update user" },
      { method: "DELETE", path: "/api/v1/users/:id", description: "Delete user" },
    ],
  },
  {
    category: "Audit Logs (Admin)",
    icon: ScrollText,
    endpoints: [
      { method: "GET", path: "/api/v1/audit-logs", description: "List audit logs" },
    ],
  },
];

const methodColors: Record<string, string> = {
  GET: "bg-blue-500/10 text-blue-700 border-blue-500",
  POST: "bg-green-500/10 text-green-700 border-green-500",
  PUT: "bg-yellow-500/10 text-yellow-700 border-yellow-500",
  DELETE: "bg-red-500/10 text-red-700 border-red-500",
};

export default function DocsPage() {
  return (
    <div className="space-y-6">
      {/* Header */}
      <div>
        <h1 className="text-3xl font-bold tracking-tight flex items-center gap-2">
          <Book className="h-8 w-8" />
          API Documentation
        </h1>
        <p className="text-muted-foreground">
          REST API reference for Sentinel Hub
        </p>
      </div>

      {/* Overview */}
      <Card>
        <CardHeader>
          <CardTitle>Overview</CardTitle>
          <CardDescription>
            Getting started with the Sentinel Hub API
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div>
            <h3 className="font-semibold mb-2">Base URL</h3>
            <code className="rounded bg-muted px-2 py-1 font-mono text-sm">
              http://localhost:8080/api/v1
            </code>
          </div>
          <div>
            <h3 className="font-semibold mb-2">Authentication</h3>
            <p className="text-sm text-muted-foreground">
              All endpoints except <code className="rounded bg-muted px-1">/auth/login</code> require a valid JWT token.
              Include the token in the Authorization header:
            </p>
            <code className="block rounded bg-muted px-3 py-2 font-mono text-sm mt-2">
              Authorization: Bearer &lt;access_token&gt;
            </code>
          </div>
          <div>
            <h3 className="font-semibold mb-2">Response Format</h3>
            <p className="text-sm text-muted-foreground">
              All responses are JSON. Errors return a standard format:
            </p>
            <pre className="rounded bg-muted px-3 py-2 font-mono text-sm mt-2 overflow-auto">
{`{
  "error": {
    "code": "ERROR_CODE",
    "message": "Human-readable message"
  }
}`}
            </pre>
          </div>
        </CardContent>
      </Card>

      {/* Endpoints by category */}
      {apiEndpoints.map((category) => (
        <Card key={category.category}>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <category.icon className="h-5 w-5" />
              {category.category}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-3">
              {category.endpoints.map((endpoint, index) => (
                <div
                  key={index}
                  className="flex items-center gap-3 rounded-lg border p-3"
                >
                  <Badge
                    variant="outline"
                    className={`w-16 justify-center font-mono ${methodColors[endpoint.method]}`}
                  >
                    {endpoint.method}
                  </Badge>
                  <code className="font-mono text-sm flex-1">
                    {endpoint.path}
                  </code>
                  <span className="text-sm text-muted-foreground hidden md:block">
                    {endpoint.description}
                  </span>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      ))}

      {/* gRPC API */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Code className="h-5 w-5" />
            gRPC API (Agent Communication)
          </CardTitle>
          <CardDescription>
            gRPC service for Sentinel agent communication
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div>
            <h3 className="font-semibold mb-2">Endpoint</h3>
            <code className="rounded bg-muted px-2 py-1 font-mono text-sm">
              localhost:9090
            </code>
          </div>
          <div>
            <h3 className="font-semibold mb-2">Services</h3>
            <div className="space-y-2">
              <div className="rounded-lg border p-3">
                <code className="font-mono text-sm font-semibold">FleetService</code>
                <ul className="mt-2 space-y-1 text-sm text-muted-foreground">
                  <li>• <code>Register</code> - Register agent with hub</li>
                  <li>• <code>Heartbeat</code> - Send health status</li>
                  <li>• <code>Deregister</code> - Remove agent registration</li>
                  <li>• <code>Subscribe</code> - Stream configuration updates</li>
                  <li>• <code>ReportDeployment</code> - Report deployment status</li>
                </ul>
              </div>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
