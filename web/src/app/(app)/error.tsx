"use client";

import { Button, Card } from "@/components/ui";

export default function ErrorPage({ error, reset }: { error: Error; reset: () => void }) {
  return (
    <Card className="mx-auto mt-16 max-w-md p-8 text-center">
      <h1 className="text-lg font-semibold">Something went wrong</h1>
      <p className="mt-2 text-sm text-muted-foreground">
        {/unreachable|cannot be reached/i.test(error.message) ? "The control plane cannot be reached. Check that it is running and that CONTROL_PLANE_URL points to it." : "The page could not be loaded. Try again."}
      </p>
      <Button className="mt-5" onClick={reset}>Try again</Button>
    </Card>
  );
}
