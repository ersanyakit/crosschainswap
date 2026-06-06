import { ArrowRight, LockKeyhole, RefreshCw, ShieldCheck, Terminal, UserCheck } from 'lucide-react';
import { BRAND_INITIAL, BRAND_NAME } from '../constants/brand';

interface LoginScreenProps {
  provider: string;
  isOIDCEnabled: boolean;
  isLoading: boolean;
  error?: string;
  onLogin: () => void;
  onRetry: () => void;
  onContinueSandbox: () => void;
}

export default function LoginScreen({
  provider,
  isOIDCEnabled,
  isLoading,
  error,
  onLogin,
  onRetry,
  onContinueSandbox,
}: LoginScreenProps) {
  return (
    <div className="w-full h-full bg-[#f6f8fa] dark:bg-[#040406] text-gray-800 dark:text-gray-200 flex items-center justify-center p-4 sm:p-6 select-none">
      <div className="w-full max-w-[920px] min-h-[520px] grid grid-cols-1 lg:grid-cols-[1.05fr_0.95fr] bg-white dark:bg-[#0c1015] border border-[#e1e4e8] dark:border-[#21262d] rounded-lg shadow-sm overflow-hidden">
        <div className="p-5 sm:p-7 flex flex-col justify-between bg-[#fafbfc] dark:bg-[#070b0f] border-b lg:border-b-0 lg:border-r border-[#e1e4e8] dark:border-[#21262d]">
          <div className="space-y-6">
            <div className="flex items-center gap-3">
              <div className="w-10 h-10 rounded bg-accent-1 text-white flex items-center justify-center font-display font-bold text-lg shadow-sm ide-glow">
                {BRAND_INITIAL}
              </div>
              <div>
                <div className="font-display text-sm font-bold tracking-wider text-gray-950 dark:text-gray-50">
                  {BRAND_NAME} LIMIT PROTOCOL
                </div>
                <div className="font-mono text-[10px] text-gray-400 uppercase">
                  OIDC secured exchange terminal
                </div>
              </div>
            </div>

            <div className="space-y-3">
              <div className="inline-flex items-center gap-1.5 px-2 py-1 rounded border border-accent-1/20 bg-accent-2 text-accent-1 font-mono text-[10px] font-bold uppercase">
                <LockKeyhole className="w-3.5 h-3.5" />
                Identity required
              </div>

              <h1 className="font-display text-2xl sm:text-3xl font-semibold tracking-normal text-gray-950 dark:text-gray-50 leading-tight">
                Secure operator session
              </h1>

              <p className="text-xs sm:text-sm text-gray-500 dark:text-gray-400 leading-6 max-w-[520px]">
                Access to order placement, balances, wallets and account history is bound to the authenticated OIDC subject. Market data remains visible only after the terminal establishes a trusted session.
              </p>
            </div>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-3 gap-2.5 font-mono text-[10px] mt-8">
            <div className="border border-[#e1e4e8] dark:border-[#21262d] bg-white dark:bg-[#0c1015] rounded p-3">
              <ShieldCheck className="w-4 h-4 text-trade-green mb-2" />
              <div className="font-bold text-gray-800 dark:text-gray-100">Session Cookie</div>
              <div className="text-gray-400 mt-1 leading-4">HTTP-only exchange session</div>
            </div>
            <div className="border border-[#e1e4e8] dark:border-[#21262d] bg-white dark:bg-[#0c1015] rounded p-3">
              <UserCheck className="w-4 h-4 text-accent-1 mb-2" />
              <div className="font-bold text-gray-800 dark:text-gray-100">Subject Binding</div>
              <div className="text-gray-400 mt-1 leading-4">OIDC sub becomes user_id</div>
            </div>
            <div className="border border-[#e1e4e8] dark:border-[#21262d] bg-white dark:bg-[#0c1015] rounded p-3">
              <Terminal className="w-4 h-4 text-sky-500 mb-2" />
              <div className="font-bold text-gray-800 dark:text-gray-100">Audit Ready</div>
              <div className="text-gray-400 mt-1 leading-4">Orders reject impersonation</div>
            </div>
          </div>
        </div>

        <div className="p-5 sm:p-7 flex flex-col justify-center">
          <div className="bg-[#fafbfc] dark:bg-[#090d12] border border-[#e1e4e8] dark:border-[#21262d] rounded-lg p-4 sm:p-5 space-y-4 shadow-xs">
            <div className="flex items-center justify-between border-b border-[#e1e4e8] dark:border-[#21262d] pb-3">
              <div>
                <div className="font-display text-sm font-semibold text-gray-900 dark:text-gray-100">
                  Identity Provider
                </div>
                <div className="font-mono text-[10px] text-gray-400 mt-0.5">
                  {provider || 'OIDC provider'}
                </div>
              </div>
              <span className={`font-mono text-[9px] px-2 py-1 rounded border ${
                isLoading
                  ? 'text-sky-600 dark:text-sky-400 bg-sky-50 dark:bg-sky-950/20 border-sky-300/40 dark:border-sky-800/40'
                  : isOIDCEnabled
                  ? 'text-trade-green bg-trade-green-bg border-trade-green/25'
                  : 'text-amber-600 dark:text-amber-400 bg-amber-50 dark:bg-amber-950/20 border-amber-300/40 dark:border-amber-800/40'
              }`}>
                {isLoading ? 'CHECKING' : isOIDCEnabled ? 'ONLINE' : 'NOT CONFIGURED'}
              </span>
            </div>

            <button
              type="button"
              onClick={onLogin}
              disabled={!isOIDCEnabled || isLoading}
              className={`w-full h-11 rounded text-xs font-mono font-bold uppercase tracking-wider flex items-center justify-center gap-2 transition-all ${
                isOIDCEnabled && !isLoading
                  ? 'bg-accent-1 hover:bg-accent-1-hovered text-white cursor-pointer shadow-md hover:shadow-accent-1/20'
                  : 'bg-gray-100 dark:bg-[#161b22] text-gray-400 border border-[#e1e4e8] dark:border-[#21262d] cursor-not-allowed'
              }`}
            >
              {isLoading ? <RefreshCw className="w-4 h-4 animate-spin" /> : <ArrowRight className="w-4 h-4" />}
              Start OIDC Login
            </button>

            {error && (
              <div className="rounded border border-rose-200 dark:border-rose-900 bg-rose-50 dark:bg-rose-950/20 text-rose-600 dark:text-rose-400 p-3 text-[10px] font-mono leading-5">
                {error}
              </div>
            )}

            {!isLoading && !isOIDCEnabled && (
              <button
                type="button"
                onClick={onContinueSandbox}
                className="w-full h-9 rounded border border-[#e1e4e8] dark:border-[#21262d] bg-white dark:bg-[#0c1015] hover:border-accent-1 text-gray-600 dark:text-gray-300 hover:text-accent-1 transition-colors text-[10px] font-mono font-bold uppercase cursor-pointer"
              >
                Continue in local sandbox
              </button>
            )}

            <button
              type="button"
              onClick={onRetry}
              className="w-full h-8 rounded text-[10px] font-mono text-gray-400 hover:text-gray-700 dark:hover:text-gray-200 transition-colors cursor-pointer"
            >
              Refresh auth status
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
