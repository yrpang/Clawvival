import type { HTMLAttributes } from "react";
import { cn } from "../lib/cn";

type CardProps = HTMLAttributes<HTMLDivElement>;

export function Card({ className, ...props }: CardProps) {
  return (
    <div
      className={cn(
        "rounded-[14px] border border-[var(--line)] bg-[var(--panel)] p-3",
        "[.theme-night_&]:border-[#2f3a5a] [.theme-night_&]:bg-[linear-gradient(180deg,rgba(21,28,44,0.96),rgba(26,30,52,0.92))]",
        className,
      )}
      {...props}
    />
  );
}
