import { Component, ErrorInfo, ReactNode } from 'react';
import { AlertTriangle, Home, RotateCw } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { useI18n } from '@/i18n/I18nContext';

interface Labels {
  title: string;
  subtitle: string;
  retry: string;
  reload: string;
  home: string;
}

interface Props {
  children: ReactNode;
  fallback?: ReactNode;
  labels: Labels;
}

interface State {
  hasError: boolean;
  error: Error | null;
  errorInfo: ErrorInfo | null;
}

/**
 * Catches render errors and shows a themed recovery screen instead of a blank
 * page. The class holds the boundary logic; the exported wrapper supplies the
 * translated labels.
 */
class ErrorBoundaryBase extends Component<Props, State> {
  constructor(props: Props) {
    super(props);
    this.state = { hasError: false, error: null, errorInfo: null };
  }

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error, errorInfo: null };
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    console.error('ErrorBoundary caught an error:', error, errorInfo);
    this.setState({ error, errorInfo });
  }

  handleReset = () =>
    this.setState({ hasError: false, error: null, errorInfo: null });

  handleReload = () => window.location.reload();

  handleHome = () => {
    window.location.href = '/manager';
  };

  render() {
    const { labels } = this.props;
    if (!this.state.hasError) return this.props.children;
    if (this.props.fallback) return this.props.fallback;

    return (
      <div className="flex min-h-[60vh] items-center justify-center p-4">
        <div className="w-full max-w-md rounded-xl border border-sidebar-border bg-card p-8 shadow-sm">
          <div className="mb-4 flex justify-center">
            <div className="rounded-full bg-destructive/10 p-3">
              <AlertTriangle className="h-7 w-7 text-destructive" />
            </div>
          </div>

          <h1 className="text-center text-xl font-semibold text-foreground">
            {labels.title}
          </h1>
          <p className="mt-1 text-center text-sm text-muted-foreground">
            {labels.subtitle}
          </p>

          {this.state.error?.message && (
            <div className="mt-5 rounded-md border border-destructive/30 bg-destructive/10 p-3">
              <p className="text-sm text-destructive">
                {this.state.error.message}
              </p>
              {import.meta.env.DEV && this.state.errorInfo && (
                <pre className="mt-2 max-h-40 overflow-auto text-xs text-destructive/80">
                  {this.state.errorInfo.componentStack}
                </pre>
              )}
            </div>
          )}

          <div className="mt-6 grid gap-2 sm:grid-cols-2">
            <Button onClick={this.handleReset}>
              <RotateCw className="h-4 w-4" />
              {labels.retry}
            </Button>
            <Button variant="outline" onClick={this.handleReload}>
              {labels.reload}
            </Button>
            <Button
              variant="ghost"
              className="sm:col-span-2"
              onClick={this.handleHome}
            >
              <Home className="h-4 w-4" />
              {labels.home}
            </Button>
          </div>
        </div>
      </div>
    );
  }
}

export default function ErrorBoundary({
  children,
  fallback,
}: {
  children: ReactNode;
  fallback?: ReactNode;
}) {
  const { t } = useI18n();
  return (
    <ErrorBoundaryBase
      fallback={fallback}
      labels={{
        title: t('error.title'),
        subtitle: t('error.subtitle'),
        retry: t('common.retry'),
        reload: t('common.reload'),
        home: t('common.home'),
      }}
    >
      {children}
    </ErrorBoundaryBase>
  );
}
