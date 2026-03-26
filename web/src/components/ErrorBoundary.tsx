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
          <div className="max-w-md rounded-xl border border-edge bg-surface p-8 text-center">
            <AlertTriangle className="mx-auto mb-4 h-12 w-12 text-red-400" />
            <h1 className="mb-2 text-xl font-bold text-fg">
              Something went wrong
            </h1>
            <p className="mb-4 text-sm text-fg-3">
              {this.state.error?.message ?? "An unexpected error occurred."}
            </p>
            <button
              onClick={() => window.location.reload()}
              className="rounded-lg bg-accent-bold px-5 py-2 text-sm font-medium text-page transition-colors hover:bg-accent-hover"
            >
              Reload Page
            </button>
          </div>
        </div>
      );
    }

    return this.props.children;
  }
}
