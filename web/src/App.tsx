import { Routes, Route } from "react-router";
import ProtectedRoute from "./components/ProtectedRoute";
import AppLayout from "./layouts/AppLayout";
import Login from "./pages/Login";
import Dashboard from "./pages/Dashboard";
import SessionList from "./pages/SessionList";
import NewSession from "./pages/NewSession";
import SessionDetail from "./pages/SessionDetail";
import WorkflowList from "./pages/WorkflowList";
import WorkflowCreate from "./pages/WorkflowCreate";
import WorkflowDetail from "./pages/WorkflowDetail";
import WorkflowRunDetail from "./pages/WorkflowRunDetail";
import Settings from "./pages/Settings";

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />

      <Route element={<ProtectedRoute />}>
        <Route element={<AppLayout />}>
          <Route path="/" element={<Dashboard />} />
          <Route path="/sessions" element={<SessionList />} />
          <Route path="/sessions/new" element={<NewSession />} />
          <Route path="/sessions/:id" element={<SessionDetail />} />
          <Route path="/workflows" element={<WorkflowList />} />
          <Route path="/workflows/new" element={<WorkflowCreate />} />
          <Route
            path="/workflows/runs/:runId"
            element={<WorkflowRunDetail />}
          />
          <Route path="/workflows/:name" element={<WorkflowDetail />} />
          <Route path="/settings" element={<Settings />} />
        </Route>
      </Route>
    </Routes>
  );
}
