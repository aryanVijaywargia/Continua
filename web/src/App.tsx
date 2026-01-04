import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { BrowserRouter, Routes, Route } from 'react-router-dom';

const queryClient = new QueryClient();

export function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <Routes>
          <Route path="/" element={<HomePage />} />
          <Route path="/traces" element={<TracesPage />} />
          <Route path="/traces/:id" element={<TraceDetailPage />} />
        </Routes>
      </BrowserRouter>
    </QueryClientProvider>
  );
}

function HomePage() {
  return (
    <div className="min-h-screen bg-gray-50 flex items-center justify-center">
      <div className="text-center">
        <h1 className="text-4xl font-bold text-gray-900">Continua</h1>
        <p className="mt-2 text-gray-600">AI Agent Observability Platform</p>
        <a href="/traces" className="mt-4 inline-block text-blue-600 hover:underline">
          View Traces →
        </a>
      </div>
    </div>
  );
}

function TracesPage() {
  return <div className="p-8"><h1 className="text-2xl font-bold">Traces</h1></div>;
}

function TraceDetailPage() {
  return <div className="p-8"><h1 className="text-2xl font-bold">Trace Detail</h1></div>;
}
