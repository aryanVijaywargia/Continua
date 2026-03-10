import { useState } from 'react';
import { getApiKey } from '../api/client';
import { ApiKeyPrompt } from '../components/ApiKeyPrompt';

export function useRequireApiKey() {
  const [hasApiKey, setHasApiKey] = useState(() => !!getApiKey());

  return {
    hasApiKey,
    prompt: hasApiKey ? null : <ApiKeyPrompt onSubmit={() => setHasApiKey(true)} />,
  };
}
