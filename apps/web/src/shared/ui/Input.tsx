import type { InputHTMLAttributes } from "react";
import { cn } from "../lib/cn";

type InputProps = InputHTMLAttributes<HTMLInputElement>;

export function Input({ className, ...props }: InputProps) {
  return (
    <input
      className={cn(
        "w-full min-w-0 rounded-[10px] border border-[var(--line)] bg-white px-3 py-2 text-inherit",
        "[.theme-night_&]:border-[#354469] [.theme-night_&]:bg-[#1e2740] [.theme-night_&]:text-[#e6eefc]",
        className,
      )}
      {...props}
    />
  );
}
