import { usePreviewData } from "../contexts/user-context"
export function usePublicConfig() { return { config: usePreviewData().data?.public_config ?? null } }
