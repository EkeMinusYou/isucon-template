import { useEffect, useRef, useState } from 'react'

type Matrix = {
  a: number
  b: number
  c: number
  d: number
  e: number
  f: number
}

type GraphElements = {
  svg: SVGSVGElement
  viewport: SVGGElement
}

type PprofGraphViewerProps = {
  src: string
  title: string
  description: string
}

const ZOOM_FACTOR = 1.2

function multiply(left: Matrix, right: Matrix): Matrix {
  return {
    a: left.a * right.a + left.c * right.b,
    b: left.b * right.a + left.d * right.b,
    c: left.a * right.c + left.c * right.d,
    d: left.b * right.c + left.d * right.d,
    e: left.a * right.e + left.c * right.f + left.e,
    f: left.b * right.e + left.d * right.f + left.f,
  }
}

function matrixFromElement(viewport: SVGGElement): Matrix {
  const matrix = viewport.transform.baseVal.consolidate()?.matrix
  return matrix
    ? { a: matrix.a, b: matrix.b, c: matrix.c, d: matrix.d, e: matrix.e, f: matrix.f }
    : { a: 1, b: 0, c: 0, d: 1, e: 0, f: 0 }
}

function matrixString(matrix: Matrix): string {
  return `matrix(${matrix.a},${matrix.b},${matrix.c},${matrix.d},${matrix.e},${matrix.f})`
}

function graphElements(iframe: HTMLIFrameElement): GraphElements | null {
  const document = iframe.contentDocument
  const svg = document?.documentElement
  const viewport = document?.getElementById('viewport')
  if (!svg || svg.tagName !== 'svg' || !viewport || viewport.tagName !== 'g') return null
  return { svg: svg as unknown as SVGSVGElement, viewport: viewport as unknown as SVGGElement }
}

function zoomViewport(elements: GraphElements, factor: number) {
  const { svg, viewport } = elements
  const current = matrixFromElement(viewport)
  const currentTransform = viewport.getCTM()
  const focus = svg.createSVGPoint()
  focus.x = svg.clientWidth / 2
  focus.y = svg.clientHeight / 2
  const localFocus = currentTransform ? focus.matrixTransform(currentTransform.inverse()) : focus
  const scale = {
    a: factor,
    b: 0,
    c: 0,
    d: factor,
    e: localFocus.x * (1 - factor),
    f: localFocus.y * (1 - factor),
  }
  viewport.setAttribute('transform', matrixString(multiply(current, scale)))
}

function fitViewport({ svg, viewport }: GraphElements) {
  const bounds = viewport.getBBox()
  const width = svg.clientWidth
  const height = svg.clientHeight
  if (bounds.width <= 0 || bounds.height <= 0 || width <= 0 || height <= 0) return

  const padding = 24
  const factor = Math.min(
    (width - padding * 2) / bounds.width,
    (height - padding * 2) / bounds.height,
  )
  viewport.setAttribute(
    'transform',
    matrixString({
      a: factor,
      b: 0,
      c: 0,
      d: factor,
      e: (width - bounds.width * factor) / 2 - bounds.x * factor,
      f: (height - bounds.height * factor) / 2 - bounds.y * factor,
    }),
  )
}

function GraphButton({
  label,
  onClick,
  disabled = false,
}: {
  label: string
  onClick: () => void
  disabled?: boolean
}) {
  return (
    <button
      type="button"
      className="btn btn-ghost btn-sm min-w-9 px-2"
      aria-label={label}
      title={label}
      disabled={disabled}
      onClick={onClick}
    >
      {label === '拡大' ? '+' : label === '縮小' ? '−' : label}
    </button>
  )
}

export function PprofGraphViewer({ src, title, description }: PprofGraphViewerProps) {
  const iframeRef = useRef<HTMLIFrameElement>(null)
  const initialTransform = useRef<string | null>(null)
  const [loadedSrc, setLoadedSrc] = useState<string | null>(null)
  const [expanded, setExpanded] = useState(false)
  const ready = loadedSrc === src

  useEffect(() => {
    initialTransform.current = null
  }, [src])

  useEffect(() => {
    if (!expanded) return
    const previousOverflow = document.body.style.overflow
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setExpanded(false)
    }
    document.body.style.overflow = 'hidden'
    window.addEventListener('keydown', onKeyDown)
    return () => {
      document.body.style.overflow = previousOverflow
      window.removeEventListener('keydown', onKeyDown)
    }
  }, [expanded])

  const onLoad = () => {
    const iframe = iframeRef.current
    if (!iframe) return
    const elements = graphElements(iframe)
    if (!elements) return
    initialTransform.current = elements.viewport.getAttribute('transform')
    setLoadedSrc(src)
  }

  const changeZoom = (factor: number) => {
    const iframe = iframeRef.current
    if (!iframe) return
    const elements = graphElements(iframe)
    if (!elements) return
    zoomViewport(elements, factor)
  }

  const reset = () => {
    const iframe = iframeRef.current
    if (!iframe) return
    const elements = graphElements(iframe)
    if (!elements) return
    elements.viewport.setAttribute('transform', initialTransform.current ?? 'matrix(1,0,0,1,0,0)')
  }

  const fit = () => {
    const iframe = iframeRef.current
    if (!iframe) return
    const elements = graphElements(iframe)
    if (!elements) return
    fitViewport(elements)
  }

  return (
    <div className="mt-6">
      {expanded && <div className="fixed inset-0 z-40 bg-black/50" aria-hidden="true" />}
      <div
        className={expanded
          ? 'fixed inset-4 z-50 flex min-h-0 flex-col gap-3 rounded-box border border-base-300 bg-base-200 p-3 shadow-2xl'
          : 'relative'}
      >
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div>
            <div className="text-sm font-semibold">{title}</div>
            <div className="text-sm text-base-content/60">{description}</div>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-xs text-base-content/55">ドラッグで移動 / ホイールでズーム</span>
            <div className="join border border-base-300 bg-base-100">
              <GraphButton label="縮小" disabled={!ready} onClick={() => changeZoom(1 / ZOOM_FACTOR)} />
              <GraphButton label="拡大" disabled={!ready} onClick={() => changeZoom(ZOOM_FACTOR)} />
              <GraphButton label="全体表示" disabled={!ready} onClick={fit} />
              <GraphButton label="リセット" disabled={!ready} onClick={reset} />
            </div>
            <button
              type="button"
              className="btn btn-outline btn-sm"
              onClick={() => setExpanded((current) => !current)}
            >
              {expanded ? '閉じる' : '拡大表示'}
            </button>
            <a
              className="btn btn-ghost btn-sm"
              href={src}
              target="_blank"
              rel="noreferrer"
            >
              別タブ
            </a>
          </div>
        </div>
        <div className={`overflow-hidden rounded-box border border-base-300 bg-base-100 ${expanded ? 'min-h-0 flex-1' : 'h-[720px]'}`}>
          <iframe
            ref={iframeRef}
            title={title}
            src={src}
            loading="lazy"
            className="block h-full w-full"
            onLoad={onLoad}
          />
        </div>
      </div>
    </div>
  )
}
