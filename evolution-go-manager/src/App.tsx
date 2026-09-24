import { lazy, Suspense } from 'react';
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { Toaster } from 'sonner';
import ErrorBoundary from '@/components/base/ErrorBoundary';
import Layout from '@/components/base/Layout';
import Home from '@/pages/Home';
import Login from '@/pages/Login';
import useAuth from '@/hooks/useAuth';
import { DarkModeProvider } from '@/contexts/ThemeContext';

// The pages behind the login are code-split, so the public Home/Login stay
// small and each screen is loaded on demand.
const Dashboard = lazy(() => import('@/pages/Dashboard'));
const Instances = lazy(() => import('@/pages/Instances'));
const InstanceSettings = lazy(() => import('@/pages/InstanceSettings'));
const Messages = lazy(() => import('@/pages/Messages'));
const Events = lazy(() => import('@/pages/Events'));
const Settings = lazy(() => import('@/pages/Settings'));
const ApiTester = lazy(() => import('@/pages/ApiTester'));
const About = lazy(() => import('@/pages/About'));
const Terms = lazy(() => import('@/pages/Terms'));
const Privacy = lazy(() => import('@/pages/Privacy'));

function PageFallback() {
  return (
    <div className="flex h-full items-center justify-center p-10">
      <span className="text-sm text-muted-foreground">Carregando…</span>
    </div>
  );
}

function App() {
  const { isAuthenticated } = useAuth();

  // There is no license gate: a valid GLOBAL_API_KEY (isAuthenticated) is all
  // that is required. This fork removed the licensing entirely.
  return (
    <DarkModeProvider>
      <ErrorBoundary>
        <BrowserRouter>
          <Suspense fallback={<PageFallback />}>
            <Routes>
              {/* Landing Page - Public */}
              <Route path="/" element={<Home />} />

              {/* Manager Login - Public */}
              <Route
                path="/manager/login"
                element={!isAuthenticated ? <Login /> : <Navigate to="/manager" replace />}
              />

              {/* Public legal pages, linked from the login screen. Kept under
                  /manager so the Go server's /manager/*any route serves the SPA. */}
              <Route path="/manager/terms" element={<Terms />} />
              <Route path="/manager/privacy" element={<Privacy />} />

              {/* Manager Protected Routes - require a valid API key */}
              {isAuthenticated ? (
                <Route path="/manager" element={<Layout />}>
                  <Route index element={<Dashboard />} />
                  <Route path="instances" element={<Instances />} />
                  <Route path="instances/:instanceId/settings" element={<InstanceSettings />} />
                  <Route path="messages" element={<Messages />} />
                  <Route path="events" element={<Events />} />
                  <Route path="api-tester" element={<ApiTester />} />
                  <Route path="settings" element={<Settings />} />
                  <Route path="about" element={<About />} />
                </Route>
              ) : (
                <Route path="/manager/*" element={<Navigate to="/manager/login" replace />} />
              )}

              {/* Catch all - redirect to home */}
              <Route path="*" element={<Navigate to="/" replace />} />
            </Routes>
          </Suspense>
        </BrowserRouter>
        <Toaster position="top-right" richColors />
      </ErrorBoundary>
    </DarkModeProvider>
  );
}

export default App;
