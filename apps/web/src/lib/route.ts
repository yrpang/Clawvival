export function normalizeSkillsPath(pathname: string): string | null {
  return pathname === "/skills" ? "/skills/" : null;
}
