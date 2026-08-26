import * as React from 'react'
import * as DialogPrimitive from '@radix-ui/react-dialog'
import { Command as CommandPrimitive } from 'cmdk'

// 上游 command.tsx 从 lucide-react 引入 SearchIcon；本仓库不依赖该包，
// 改为文件内同几何的最小内联 SVG 组件。
function SearchIcon(props: React.ComponentProps<'svg'>) {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      {...props}
    >
      <circle cx="11" cy="11" r="8" />
      <path d="m21 21-4.3-4.3" />
    </svg>
  )
}

// 命令条目：由调用方（AppShell）经 props 提供，命令面板本身不认识路由或功能模块
export type CommandItem = {
  id: string
  label: string
  group: string
  keywords?: readonly string[]
  onSelect: () => void
}

export function CommandPalette({
  commands,
  open,
  onOpenChange,
}: {
  commands: readonly CommandItem[]
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  // 按首次出现顺序聚合分组标题
  const groupNames: string[] = []
  for (const command of commands) {
    if (!groupNames.includes(command.group)) groupNames.push(command.group)
  }

  return (
    <DialogPrimitive.Root open={open} onOpenChange={onOpenChange}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Overlay
          className={
            'fixed inset-0 z-50 bg-black/50 data-[state=closed]:animate-out data-[state=closed]:fade-out-0 ' +
            'data-[state=open]:animate-in data-[state=open]:fade-in-0'
          }
        />
        <DialogPrimitive.Content
          className={
            'fixed top-[20%] left-1/2 z-50 w-full max-w-lg -translate-x-1/2 overflow-hidden ' +
            'rounded-md border bg-popover text-popover-foreground shadow-lg ' +
            'data-[state=closed]:animate-out data-[state=closed]:fade-out-0 ' +
            'data-[state=open]:animate-in data-[state=open]:fade-in-0'
          }
        >
          <DialogPrimitive.Title className="sr-only">命令面板</DialogPrimitive.Title>
          <DialogPrimitive.Description className="sr-only">
            输入以筛选并执行命令
          </DialogPrimitive.Description>
          <CommandPrimitive className="flex w-full flex-col overflow-hidden">
            <div className="flex h-9 items-center gap-2 border-b px-3">
              <SearchIcon className="size-4 shrink-0 opacity-50" />
              <CommandPrimitive.Input
                placeholder="搜索命令…"
                className="flex h-10 w-full rounded-md bg-transparent py-3 text-sm outline-hidden placeholder:text-muted-foreground disabled:cursor-not-allowed disabled:opacity-50"
              />
            </div>
            <CommandPrimitive.List className="max-h-[300px] scroll-py-1 overflow-x-hidden overflow-y-auto">
              <CommandPrimitive.Empty className="py-6 text-center text-sm">
                没有匹配的结果
              </CommandPrimitive.Empty>
              {groupNames.map((groupName) => (
                <CommandPrimitive.Group
                  key={groupName}
                  heading={groupName}
                  className={
                    'overflow-hidden p-1 text-foreground [&_[cmdk-group-heading]]:px-2 ' +
                    '[&_[cmdk-group-heading]]:py-1.5 [&_[cmdk-group-heading]]:text-xs ' +
                    '[&_[cmdk-group-heading]]:font-medium [&_[cmdk-group-heading]]:text-muted-foreground'
                  }
                >
                  {commands
                    .filter((command) => command.group === groupName)
                    .map((command) => (
                      <CommandPrimitive.Item
                        key={command.id}
                        value={command.label}
                        keywords={command.keywords ? [...command.keywords] : undefined}
                        onSelect={() => command.onSelect()}
                        className={
                          'relative flex cursor-default items-center gap-2 rounded-sm px-2 py-1.5 text-sm ' +
                          'outline-hidden select-none data-[disabled=true]:pointer-events-none ' +
                          'data-[disabled=true]:opacity-50 data-[selected=true]:bg-accent ' +
                          'data-[selected=true]:text-accent-foreground'
                        }
                      >
                        {command.label}
                      </CommandPrimitive.Item>
                    ))}
                </CommandPrimitive.Group>
              ))}
            </CommandPrimitive.List>
          </CommandPrimitive>
        </DialogPrimitive.Content>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  )
}
