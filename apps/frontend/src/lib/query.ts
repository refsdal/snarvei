import { QueryClient } from "@tanstack/react-query";
import { ApiError } from "./api";

// 4xx answers are final (a 401 means "not signed in", a 404 "gone"); only
// network and 5xx failures are worth one retry.
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 15_000,
      retry: (count, err) => count < 1 && !(err instanceof ApiError && err.status < 500),
      refetchOnWindowFocus: false,
    },
  },
});

// Called whenever the identity behind the cache changes (sign in/out, account
// deletion): nothing of the previous user may survive.
export async function resetCache(): Promise<void> {
  queryClient.clear();
}
