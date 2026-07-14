declare module 'hast' {
  export interface Element {
    children: Array<Element | { type: string; value?: string }>
    properties?: Record<string, unknown>
    tagName: string
    type: string
  }
}
