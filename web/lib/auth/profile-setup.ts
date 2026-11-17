import { safeReturnTo } from "@/lib/api"

export function afterProfileSetup(returnTo: string | null) {
  const path = safeReturnTo(returnTo)
  return path.split(/[?#]/, 1)[0].replace(/\/+$/, "") === "/welcome" ? "/account" : path
}

export function profileSetupPath(returnTo: string | null) {
  return `/welcome?return_to=${encodeURIComponent(afterProfileSetup(returnTo))}`
}
