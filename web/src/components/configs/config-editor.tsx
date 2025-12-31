"use client";

import { useCallback } from "react";
import Editor from "@monaco-editor/react";
import { Loader2 } from "lucide-react";

interface ConfigEditorProps {
  value: string;
  onChange?: (value: string) => void;
  readOnly?: boolean;
  height?: string;
}

export function ConfigEditor({
  value,
  onChange,
  readOnly = false,
  height = "400px",
}: ConfigEditorProps) {
  const handleChange = useCallback(
    (val: string | undefined) => {
      if (onChange && val !== undefined) {
        onChange(val);
      }
    },
    [onChange]
  );

  return (
    <div className="border rounded-md overflow-hidden" style={{ height }}>
      <Editor
        height="100%"
        defaultLanguage="plaintext"
        value={value}
        onChange={handleChange}
        loading={
          <div className="flex items-center justify-center h-full bg-muted/50">
            <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
          </div>
        }
        options={{
          readOnly,
          minimap: { enabled: false },
          fontSize: 14,
          fontFamily: "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace",
          lineNumbers: "on",
          scrollBeyondLastLine: false,
          wordWrap: "on",
          tabSize: 4,
          insertSpaces: true,
          automaticLayout: true,
          padding: { top: 12, bottom: 12 },
        }}
        theme="vs-dark"
      />
    </div>
  );
}
