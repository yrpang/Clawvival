import type { HTMLAttributes } from "react";
import { cn } from "../lib/cn";

type BadgeProps = HTMLAttributes<HTMLSpanElement> & {
  tone?: "neutral" | "ok" | "failed";
};

export function Badge({ className, tone = "neutral", ...props }: BadgeProps) {
  return (
    <span
      className={cn(
        "inline-flex w-fit items-center justify-center whitespace-nowrap rounded-full border px-2 py-0.5 leading-tight",
        "border-[#d7cab8]",
        tone === "ok" && "border-[#b8dfc8] text-[var(--good)] [.theme-night_&]:border-[#67bca2] [.theme-night_&]:text-[#9de5cf]",
        tone === "failed" && "border-[#e8b9b9] text-[var(--critical)] [.theme-night_&]:border-[#d68e8e] [.theme-night_&]:text-[#ffc2c2]",
        tone === "neutral" && "[.theme-night_&]:border-[#5b6f9e] [.theme-night_&]:bg-[#253150] [.theme-night_&]:text-[#e8f1ff]",
        className,
      )}
      {...props}
    />
  );
}
