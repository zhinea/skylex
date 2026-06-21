export type SettingType = "number" | "memory" | "enum";

interface SettingInputProps {
  id: string;
  label: string;
  type: SettingType;
  value: string;
  onChange: (value: string) => void;
  hint?: string;
  options?: string[];
  min?: number;
  disabled?: boolean;
}

const settingLabels: Record<string, string> = {
  max_connections: "Max connections",
  shared_buffers: "Shared buffers",
  wal_level: "WAL level",
  max_wal_senders: "Max WAL senders",
  work_mem: "Work memory",
};

export const curatedSettings: Array<{
  key: string;
  label: string;
  type: SettingType;
  hint: string;
  options?: string[];
}> = [
  {
    key: "max_connections",
    label: "Max connections",
    type: "number",
    hint: "Positive integer (requires restart).",
  },
  {
    key: "shared_buffers",
    label: "Shared buffers",
    type: "memory",
    hint: "Memory value such as 128MB or 1GB (requires restart).",
  },
  {
    key: "wal_level",
    label: "WAL level",
    type: "enum",
    hint: "replica or logical (requires restart).",
    options: ["replica", "logical"],
  },
  {
    key: "max_wal_senders",
    label: "Max WAL senders",
    type: "number",
    hint: "Positive integer (requires restart).",
  },
  {
    key: "work_mem",
    label: "Work memory",
    type: "memory",
    hint: "Memory value such as 4MB (reloadable).",
  },
];

export function getSettingLabel(key: string): string {
  return settingLabels[key] || key;
}

export function validateSettingValue(key: string, value: string): string | null {
  if (!value.trim()) {
    return "Value is required";
  }
  switch (key) {
    case "max_connections":
    case "max_wal_senders": {
      const n = Number(value.trim());
      if (!Number.isInteger(n) || n <= 0) {
        return "Must be a positive integer";
      }
      return null;
    }
    case "shared_buffers":
    case "work_mem": {
      if (!/^\d+\s*(kB|MB|GB|TB|k|m|g|t)?$/i.test(value.trim())) {
        return "Must be a memory value such as 128MB";
      }
      return null;
    }
    case "wal_level": {
      if (!["replica", "logical"].includes(value.trim().toLowerCase())) {
        return "Must be replica or logical";
      }
      return null;
    }
    default:
      return null;
  }
}

export function SettingInput({
  label,
  type,
  value,
  onChange,
  hint,
  options,
  min,
  disabled,
}: SettingInputProps) {
  const commonClasses =
    "w-full px-3 py-2 text-sm border rounded-md bg-transparent " +
    "border-input text-foreground transition-all " +
    "focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50";

  return (
    <div>
      <label className="block text-xs font-semibold uppercase tracking-wider text-muted-foreground mb-1.5">
        {label}
      </label>
      {type === "enum" && options ? (
        <select
          value={value}
          onChange={(e) => onChange(e.target.value)}
          disabled={disabled}
          className={commonClasses}
        >
          {options.map((opt) => (
            <option key={opt} value={opt} className="bg-popover text-popover-foreground">
              {opt}
            </option>
          ))}
        </select>
      ) : (
        <input
          type={type === "number" ? "number" : "text"}
          min={min}
          value={value}
          onChange={(e) => onChange(e.target.value)}
          disabled={disabled}
          className={commonClasses}
        />
      )}
      {hint && <p className="mt-1.5 text-xs text-muted-foreground">{hint}</p>}
    </div>
  );
}
