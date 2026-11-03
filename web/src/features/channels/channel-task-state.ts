/** Saved-read feedback only; a completed read never asserts channel health. */
export type ChannelEvidenceTask = 'quota' | 'events' | 'routing'

export type ChannelTaskReadState = {
  phase: 'unread' | 'loading' | 'ready' | 'error'
  /** Unix seconds of the MyAPI saved-data read, not a provider observation. */
  readAt?: number
  incomplete?: boolean
  /** Failed observations in loaded evidence, distinct from a failed HTTP read. */
  failedObservations?: boolean
  samplingEnabled?: boolean
}

export type ChannelTaskProps = {
  /** Undefined preserves the component's existing standalone interaction. */
  active?: boolean
  onReadStateChange?: (state: ChannelTaskReadState) => void
}
