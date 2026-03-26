import { useEffect } from "react";

export function usePageTitle(title: string) {
  useEffect(() => {
    document.title = `CodeForge - ${title}`;
    return () => {
      document.title = "CodeForge";
    };
  }, [title]);
}
