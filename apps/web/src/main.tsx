import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import "./tailwind.css";
import "./index.css";
import App from "./App";
import { normalizeSkillsPath } from "./lib/route";

const queryClient = new QueryClient();
const redirectTo = normalizeSkillsPath(window.location.pathname);
if (redirectTo !== null) {
  window.location.replace(redirectTo);
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <App />
    </QueryClientProvider>
  </StrictMode>,
);
