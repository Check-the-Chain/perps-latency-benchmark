import { useEffect, useRef, useState } from "react"

export function useNearViewport<T extends Element>(rootMargin = "720px") {
  const ref = useRef<T | null>(null)
  const [isNear, setIsNear] = useState(false)

  useEffect(() => {
    const node = ref.current
    if (!node || isNear) {
      return
    }

    if (typeof IntersectionObserver === "undefined") {
      setIsNear(true)
      return
    }

    const observer = new IntersectionObserver(
      ([entry]) => {
        if (!entry?.isIntersecting) {
          return
        }
        setIsNear(true)
        observer.disconnect()
      },
      { rootMargin }
    )
    observer.observe(node)

    return () => observer.disconnect()
  }, [isNear, rootMargin])

  return [ref, isNear] as const
}
