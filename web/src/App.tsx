import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { BrowserRouter, Routes, Route } from 'react-router-dom';
import { TracesPage } from './pages/TracesPage';
import { TraceDetailPage } from './pages/TraceDetailPage';
import { SessionsPage } from './pages/SessionsPage';
import { SessionDetailPage } from './pages/SessionDetailPage';
import { SessionComparePage } from './pages/SessionComparePage';
import { SettingsPage } from './pages/SettingsPage';
import { ProjectsPage } from './pages/ProjectsPage';
import { ThemeProvider } from './hooks/ThemeProvider';
import { AppShell } from './components/AppShell';
import { OverviewPage } from './pages/OverviewPage';
import { LandingPage } from './pages/LandingPage';
import {
  Auth0RuntimeProvider,
  ConsoleRoute,
  useRuntimeAuthState,
} from './auth/runtime';

const queryClient = new QueryClient();

export function App() {
  const auth = useRuntimeAuthState();

  return (
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <BrowserRouter>
          <Auth0RuntimeProvider auth={auth}>
            <Routes>
              <Route path="/" element={<LandingPage />} />
              <Route element={<ConsoleRoute auth={auth} />}>
                <Route element={<AppShell />}>
                  <Route path="/dashboard" element={<OverviewPage />} />
                  <Route path="/traces" element={<TracesPage />} />
                  <Route path="/traces/:id" element={<TraceDetailPage />} />
                  <Route path="/sessions" element={<SessionsPage />} />
                  <Route path="/sessions/:id" element={<SessionDetailPage />} />
                  <Route path="/sessions/:id/compare" element={<SessionComparePage />} />
                  <Route path="/projects" element={<ProjectsPage />} />
                  <Route path="/settings" element={<SettingsPage />} />
                </Route>
              </Route>
            </Routes>
          </Auth0RuntimeProvider>
        </BrowserRouter>
      </ThemeProvider>
    </QueryClientProvider>
  );
}
