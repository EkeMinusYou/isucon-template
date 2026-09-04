import { useEffect, useRef, useState } from 'react'

export type SectionDef = {
  id: string
  title: string
  navLabel: string
}

type Props = {
  sections: SectionDef[]
  /** sticky ヘッダーの高さ。スクロール位置の判定に使う */
  offset: number
}

/**
 * ダッシュボードが縦に長いので、各セクションへ一発で飛べるジャンプバー。
 * スクロールに追従して現在のセクションをハイライトする。
 */
export function SectionNav({ sections, offset }: Props) {
  const [activeId, setActiveId] = useState<string | null>(sections[0]?.id ?? null)
  const chipRefs = useRef<Record<string, HTMLButtonElement | null>>({})

  useEffect(() => {
    let frame = 0
    const update = () => {
      frame = 0
      const line = offset + 24
      let current = sections[0]?.id ?? null
      for (const section of sections) {
        const el = document.getElementById(section.id)
        if (!el) continue
        if (el.getBoundingClientRect().top <= line) current = section.id
      }
      // 最下部まで来たら最後のセクションを選ぶ（短いセクションが選ばれないため）
      if (window.innerHeight + window.scrollY >= document.body.scrollHeight - 2) {
        current = sections[sections.length - 1]?.id ?? current
      }
      setActiveId(current)
    }
    const onScroll = () => {
      if (frame) return
      frame = window.requestAnimationFrame(update)
    }
    update()
    window.addEventListener('scroll', onScroll, { passive: true })
    window.addEventListener('resize', onScroll)
    return () => {
      if (frame) window.cancelAnimationFrame(frame)
      window.removeEventListener('scroll', onScroll)
      window.removeEventListener('resize', onScroll)
    }
  }, [sections, offset])

  // ハイライトされたチップがジャンプバーの外にあるときは見える位置まで寄せる
  useEffect(() => {
    if (!activeId) return
    chipRefs.current[activeId]?.scrollIntoView({ block: 'nearest', inline: 'nearest' })
  }, [activeId])

  const jumpTo = (id: string) => {
    // section 側の scroll-margin-top が sticky ヘッダー分を吸収する
    document.getElementById(id)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  }

  return (
    <div className="flex items-center gap-3 border-t border-base-300 px-8 py-2">
      <div className="tooltip tooltip-bottom shrink-0" data-tip="ページ先頭へ">
        <button
          className="btn btn-ghost btn-sm btn-circle"
          aria-label="ページ先頭へ"
          onClick={() => window.scrollTo({ top: 0, behavior: 'smooth' })}
        >
          <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" className="size-4">
            <path d="M12 19V5m0 0-6 6m6-6 6 6" />
          </svg>
        </button>
      </div>
      <nav aria-label="セクション" className="overflow-x-auto">
        {/* tabs-box だと active の白いピルが明色テーマで下地に溶けるので下線タイプにする */}
        <div role="tablist" className="tabs tabs-sm tabs-border w-max flex-nowrap">
          {sections.map((section) => (
            <button
              key={section.id}
              ref={(el) => {
                chipRefs.current[section.id] = el
              }}
              role="tab"
              aria-selected={activeId === section.id}
              title={section.title}
              className={`tab whitespace-nowrap ${activeId === section.id ? 'tab-active' : ''}`}
              onClick={() => jumpTo(section.id)}
            >
              {section.navLabel}
            </button>
          ))}
        </div>
      </nav>
    </div>
  )
}
