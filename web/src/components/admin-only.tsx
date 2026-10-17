import { LockIcon } from "./icons";
import { Card } from "./ui";

/** What a sales person sees on a page only admins may use. The API refuses them too. */
export function AdminOnly({ what }: { what: string }) {
  return (
    <Card className="mx-auto mt-16 max-w-md p-8 text-center">
      <LockIcon className="mx-auto size-6 text-muted-foreground" />
      <h1 className="mt-3 text-lg font-semibold">Admins only</h1>
      <p className="mt-2 text-sm text-muted-foreground">{what} is managed by an admin. Ask one of them if something here needs to change.</p>
    </Card>
  );
}
