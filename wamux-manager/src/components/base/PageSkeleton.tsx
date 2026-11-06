import { useLocation } from 'react-router-dom';

import { Skeleton } from '@/components/ui';

const LIST_WIDTH_KEY = 'wamux.messages.listWidth';
const DEFAULT_LIST_WIDTH = 340;

function times(count: number, render: (index: number) => React.ReactNode) {
  return Array.from({ length: count }, (_, index) => render(index));
}

function PageTitle({ actions = [] }: { actions?: string[] }) {
  return (
    <div className="flex flex-wrap items-start justify-between gap-3">
      <div className="space-y-2">
        <Skeleton className="h-8 w-44" />
        <Skeleton className="h-4 w-72 max-w-full" />
      </div>
      <div className="flex flex-wrap gap-2">
        {actions.map((width, i) => (
          <Skeleton key={i} className={`h-9 ${width}`} />
        ))}
      </div>
    </div>
  );
}

function Card({ className }: { className: string }) {
  return <Skeleton className={`rounded-xl ${className}`} />;
}

function DashboardSkeleton() {
  return (
    <div className="h-full overflow-hidden p-4 sm:p-6">
      <div className="mx-auto max-w-6xl space-y-6">
        <PageTitle actions={['w-20', 'w-28', 'w-44', 'w-16', 'w-9', 'w-20']} />
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          {times(8, (i) => (
            <Card key={i} className="h-28" />
          ))}
        </div>
        <Card className="h-52" />
        <div className="grid gap-4 lg:grid-cols-3">
          <Card className="h-56" />
          <Card className="h-56 lg:col-span-2" />
        </div>
      </div>
    </div>
  );
}

/** The instance grid, also shown by the page while the list loads. */
export function InstanceCardsSkeleton() {
  return (
    <div className="grid grid-cols-1 gap-6 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
      {times(4, (i) => (
        <Card key={i} className="h-80" />
      ))}
    </div>
  );
}

function InstancesSkeleton() {
  return (
    <div className="flex h-full flex-col gap-4 overflow-hidden p-4">
      <PageTitle actions={['w-9', 'w-40']} />
      <Skeleton className="h-9 w-md max-w-full" />
      <InstanceCardsSkeleton />
    </div>
  );
}

/** Also shown by the page while the instance loads. */
export function InstanceSettingsSkeleton() {
  return (
    <div className="flex h-full flex-col overflow-hidden">
      <div className="flex items-center gap-4 border-b border-sidebar-border bg-sidebar p-4 sm:p-6">
        <Skeleton className="h-9 w-9" />
        <div className="space-y-2">
          <Skeleton className="h-7 w-52" />
          <Skeleton className="h-4 w-32" />
        </div>
      </div>
      <div className="flex-1 p-4 sm:p-6">
        <div className="mx-auto max-w-4xl space-y-6">
          <Card className="h-68" />
          <Card className="h-68" />
          <Card className="h-72" />
        </div>
      </div>
    </div>
  );
}

function listWidth(): number {
  try {
    return Number(localStorage.getItem(LIST_WIDTH_KEY)) || DEFAULT_LIST_WIDTH;
  } catch {
    return DEFAULT_LIST_WIDTH;
  }
}

function MessagesSkeleton() {
  return (
    <div className="flex h-full min-h-0 flex-col overflow-hidden">
      <div className="flex items-center gap-3 border-b border-border p-3 sm:p-4">
        <Skeleton className="hidden h-7 w-28 sm:block" />
        <Skeleton className="h-9 w-64 max-w-full" />
        <Skeleton className="hidden h-9 w-32 sm:block" />
        <Skeleton className="hidden h-9 w-24 sm:block" />
      </div>
      <div className="flex min-h-0 flex-1">
        <div
          className="flex w-full shrink-0 flex-col gap-1 border-r border-border p-3 lg:w-(--list-width)"
          style={{ '--list-width': `${listWidth()}px` } as React.CSSProperties}
        >
          <Skeleton className="mb-2 h-9 w-full" />
          <div className="mb-2 flex gap-2">
            {times(4, (i) => (
              <Skeleton key={i} className="h-6 w-16 rounded-full" />
            ))}
          </div>
          {times(7, (i) => (
            <div key={i} className="flex items-center gap-3 py-2">
              <Skeleton className="h-10 w-10 shrink-0 rounded-full" />
              <div className="min-w-0 flex-1 space-y-2">
                <Skeleton className="h-3.5 w-2/3" />
                <Skeleton className="h-3 w-full" />
              </div>
            </div>
          ))}
        </div>
        <div className="hidden min-w-0 flex-1 lg:block" />
      </div>
    </div>
  );
}

function EventsSkeleton() {
  return (
    <div className="flex h-full min-h-0 flex-col gap-3 overflow-hidden p-4 sm:p-6">
      <PageTitle actions={['w-16', 'w-24', 'w-24']} />
      <div className="flex gap-2">
        <Skeleton className="h-9 w-44" />
        <Skeleton className="h-9 w-96 max-w-full" />
      </div>
      <Card className="min-h-0 flex-1" />
      <Skeleton className="h-3 w-96 max-w-full" />
    </div>
  );
}

function ApiTesterSkeleton() {
  return (
    <div className="flex h-full overflow-hidden">
      <div className="hidden w-80 shrink-0 space-y-2 border-r border-border p-3 md:block">
        <Skeleton className="mb-3 h-9 w-full" />
        {times(12, (i) => (
          <Skeleton key={i} className="h-6 w-full" />
        ))}
      </div>
      <div className="flex-1" />
    </div>
  );
}

function AboutSkeleton() {
  return (
    <div className="h-full overflow-hidden p-4 sm:p-6">
      <div className="mx-auto max-w-3xl space-y-6">
        <PageTitle />
        <Card className="h-32" />
        <Card className="h-48" />
        <Card className="h-40" />
      </div>
    </div>
  );
}

const SKELETONS: [RegExp, () => React.ReactNode][] = [
  [/^\/manager\/instances\/[^/]+\/settings/, InstanceSettingsSkeleton],
  [/^\/manager\/instances/, InstancesSkeleton],
  [/^\/manager\/messages/, MessagesSkeleton],
  [/^\/manager\/events/, EventsSkeleton],
  [/^\/manager\/api-tester/, ApiTesterSkeleton],
  [/^\/manager\/about/, AboutSkeleton],
];

/** What the page being loaded will look like, so its content lands where this was. */
function PageSkeleton() {
  const { pathname } = useLocation();
  const Match = SKELETONS.find(([path]) => path.test(pathname))?.[1] ?? DashboardSkeleton;

  return (
    <div className="h-full" role="status" aria-busy="true" data-page-skeleton={pathname}>
      <Match />
    </div>
  );
}

export default PageSkeleton;
