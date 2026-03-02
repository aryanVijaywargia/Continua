import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { BrowserRouter, Routes, Route, Link } from 'react-router-dom';
import { TracesPage } from './pages/TracesPage';
import { TraceDetailPage } from './pages/TraceDetailPage';
import { SessionsPage } from './pages/SessionsPage';
import { SessionDetailPage } from './pages/SessionDetailPage';
import { Navigation } from './components/Navigation';

const queryClient = new QueryClient();

export function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <Routes>
          <Route path="/" element={<HomePage />} />
          <Route path="/traces" element={<PageWithNav><TracesPage /></PageWithNav>} />
          <Route path="/traces/:id" element={<PageWithNav><TraceDetailPage /></PageWithNav>} />
          <Route path="/sessions" element={<PageWithNav><SessionsPage /></PageWithNav>} />
          <Route path="/sessions/:id" element={<PageWithNav><SessionDetailPage /></PageWithNav>} />
        </Routes>
      </BrowserRouter>
    </QueryClientProvider>
  );
}

function PageWithNav({ children }: { children: React.ReactNode }) {
  return (
    <>
      <Navigation />
      {children}
    </>
  );
}

function HomePage() {
  return (
    <div className="min-h-screen bg-gray-50 flex items-center justify-center">
      <div className="text-center">
        <h1 className="text-4xl font-bold text-gray-900">Continua</h1>
        <p className="mt-2 text-gray-600">AI Agent Observability Platform</p>
        <div className="mt-4 space-x-4">
          <Link to="/traces" className="inline-block text-blue-600 hover:underline">
            View Traces
          </Link>
          <Link to="/sessions" className="inline-block text-blue-600 hover:underline">
            View Sessions
          </Link>
        </div>
      </div>
    </div>
  );
}
