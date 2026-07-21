import { Component, type ReactNode } from "react";
import { AlertTriangle } from "lucide-react";

interface Props {
  children: ReactNode;
}

interface State {
  hasError: boolean;
  error: Error | null;
}

export default class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props);
    this.state = { hasError: false, error: null };
  }

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error };
  }

  render() {
    if (this.state.hasError) {
      return (
        <div className="flex min-h-screen items-center justify-center bg-page p-4">
          <div className="w-full max-w-md rounded-md border border-edge bg-surface p-8 text-center">
            <div className="mx-auto mb-4 flex size-12 items-center justify-center rounded-md border border-danger/30 bg-danger/10">
              <AlertTriangle
                className="size-6 text-danger"
                strokeWidth={1.75}
              />
            </div>
            <h1 className="mb-2 text-lg font-semibold text-fg">
              Something went wrong
            </h1>
            <p className="mb-6 text-sm text-fg-3">
              {this.state.error?.message ?? "An unexpected error occurred."}
            </p>
            <button
              onClick={() => window.location.reload()}
              className="rounded-md border border-edge bg-surface px-4 py-2 text-sm font-medium text-fg-2 transition-colors hover:border-fg-4 hover:text-fg"
            >
              Reload
            </button>
          </div>
        </div>
      );
    }

    return this.props.children;
  }
}
