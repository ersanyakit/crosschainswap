import { useState } from 'react';

interface AssetIconProps {
  symbol?: string;
  iconURL?: string;
  size?: 'xs' | 'sm' | 'md' | 'lg';
  className?: string;
}

const sizeClass = {
  xs: 'w-4 h-4 text-[8px]',
  sm: 'w-5 h-5 text-[9px]',
  md: 'w-7 h-7 text-[11px]',
  lg: 'w-10 h-10 text-sm',
};

export default function AssetIcon({
  symbol = '',
  iconURL,
  size = 'sm',
  className = '',
}: AssetIconProps) {
  const [failed, setFailed] = useState(false);
  const label = displaySymbol(symbol);
  const canRenderImage = Boolean(iconURL) && !failed;

  return (
    <span
      className={`${sizeClass[size]} ${className} inline-flex shrink-0 items-center justify-center overflow-hidden rounded-full border border-[#d8dee4] bg-white text-gray-700 shadow-xs dark:border-[#30363d] dark:bg-[#111827] dark:text-gray-100`}
      title={label}
    >
      {canRenderImage ? (
        <img
          src={iconURL}
          alt={label}
          className="h-full w-full rounded-full object-cover"
          loading="lazy"
          referrerPolicy="no-referrer"
          onError={() => setFailed(true)}
        />
      ) : (
        <span className="font-bold uppercase leading-none">
          {label.slice(0, 3)}
        </span>
      )}
    </span>
  );
}

function displaySymbol(symbol: string): string {
  const normalized = symbol.trim().toUpperCase();
  if (normalized === 'WETH') return 'ETH';
  if (normalized === 'WSOL') return 'SOL';
  if (normalized === 'WAVAX') return 'AVAX';
  if (normalized === 'WCHZ') return 'CHZ';
  return normalized || '?';
}
