<script setup lang="ts">
// A red-pencil loop drawn around a value the system flagged. When a person decides (resolved), a blue ballpoint
// tick is drawn over it: the app's single motion moment, and it only plays in answer to that action.
import { onMounted, ref, watch } from 'vue'

const props = withDefaults(defineProps<{ resolved?: boolean; tone?: 'pencil' | 'amber' }>(), {
  resolved: false,
  tone: 'pencil',
})
const animate = ref(false)
onMounted(() => {
  watch(
    () => props.resolved,
    (now, before) => {
      animate.value = now && !before
    },
  )
})
</script>

<template>
  <span class="pencil-circle" :class="tone">
    <slot />
    <svg class="loop" viewBox="0 0 200 100" preserveAspectRatio="none" aria-hidden="true">
      <path
        d="M14 58 C4 30 46 8 104 8 C162 8 196 24 193 50 C190 78 146 93 96 91 C44 89 8 76 11 52 C13 40 26 29 44 24"
        vector-effect="non-scaling-stroke"
      />
    </svg>
    <svg v-if="resolved" class="tick" :class="{ draw: animate }" viewBox="0 0 100 100" aria-hidden="true">
      <path d="M18 56 L40 78 L86 18" pathLength="1" vector-effect="non-scaling-stroke" />
    </svg>
  </span>
</template>

<style scoped>
.pencil-circle {
  position: relative;
  display: inline-block;
  white-space: nowrap;
  padding: 0.1em 0.55em;
}
.loop,
.tick {
  position: absolute;
  pointer-events: none;
  overflow: visible;
}
.loop {
  inset: -0.2em -0.15em;
  width: calc(100% + 0.3em);
  height: calc(100% + 0.4em);
}
.loop path {
  fill: none;
  stroke: var(--color-pencil);
  stroke-width: 1.8px;
  stroke-linecap: round;
  opacity: 0.9;
}
.amber .loop path {
  stroke: var(--color-amber);
}
.tick {
  right: -1.35em;
  top: -0.25em;
  width: 1.5em;
  height: 1.5em;
}
.tick path {
  fill: none;
  stroke: var(--color-ballpoint);
  stroke-width: 2.6px;
  stroke-linecap: round;
  stroke-linejoin: round;
}
.tick.draw path {
  stroke-dasharray: 1;
  animation: write 420ms ease-out both;
}
@keyframes write {
  from {
    stroke-dashoffset: 1;
  }
  to {
    stroke-dashoffset: 0;
  }
}
</style>
