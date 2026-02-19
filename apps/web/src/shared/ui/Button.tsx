import type { ButtonHTMLAttributes } from "react";
import { cn } from "../lib/cn";

type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: "primary" | "ghost";
};

export function Button({ className, variant = "primary", ...props }: ButtonProps) {
  return (
    <button
      className={cn(
        "rounded-[10px] px-3 py-2 text-inherit",
        variant === "primary" && "border border-[#b76345] bg-[linear-gradient(180deg,#f37e54,#dc5d30)] text-white",
        variant === "ghost" && "border border-[var(--line)] bg-transparent text-inherit",
        "disabled:cursor-not-allowed disabled:opacity-40",
        className,
      )}
      {...props}
    />
  );
}
