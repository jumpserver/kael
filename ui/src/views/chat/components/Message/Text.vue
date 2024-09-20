<script setup>
import { computed, onMounted, onUnmounted, onUpdated, ref, toRefs } from 'vue'
import { useMessage } from 'naive-ui'
import MarkdownIt from 'markdown-it'
import mdKatex from '@traptitech/markdown-it-katex'
import mila from 'markdown-it-link-attributes'
import hljs from 'highlight.js'
import 'highlight.js/styles/atom-one-dark.css'
import { copy } from '@/utils/common'

const props = defineProps({
  message: Object,
  asRawText: Boolean,
  error: String
})

const { error } = toRefs(props)
const NMessage = useMessage()
const textRef = ref()
const role = props.message?.role !== 'assistant'

const mdi = new MarkdownIt({
  html: false,
  linkify: true,
  highlight(code, language) {
    const validLang = !!(language && hljs.getLanguage(language))
    if (validLang) {
      const lang = language ?? ''

      return highlightBlock(hljs.highlight(lang, code, true).value, lang)
    }
    return highlightBlock(hljs.highlightAuto(code).value, '')
  },
})

mdi.use(mila, { attrs: { target: '_blank', rel: 'noopener', class: "link-style" } })
mdi.use(mdKatex, { blockClass: 'katexmath-block rounded-md p-[10px]', errorColor: ' #cc0000' })

const wrapClass = computed(() => {
  return [
    'text-wrap',
    'min-w-[20px]',
    'rounded-md',
    role ? 'bg-[#d2f9d1]' : 'bg-[#f4f6f8]',
    role ? 'dark:bg-[#a1dc95]' : 'dark:bg-[#1e1e20]',
    role ? 'message-request' : 'message-reply',
    error.value ? 'text-red-500' : ''
  ]
})

const text = computed(() => {
  const value = props.message?.content ?? ''
  if (props.message?.content) {
    return mdi.render(value)
  }
  return value
})

const onCopy = _.throttle((code)=> {
  copy(code)
  NMessage.success('复制成功')
}, 800)

function highlightBlock(str, lang) {
  return `<pre class="code-block-wrapper"><div class="code-block-header"><span class="code-block-header__lang"></span><span class="code-block-header__copy">${'Copy Code'}</span></div><code class="hljs code-block-body ${lang}">${str}</code></pre>`
}

function addCopyEvents() {
  if (textRef.value) {
    const copyBtn = textRef.value.querySelectorAll('.code-block-header__copy')
    copyBtn.forEach((btn) => {
      btn.addEventListener('click', () => {
        const code = btn.parentElement?.nextElementSibling?.textContent
        if (code) {
          onCopy(code)
        }
      })
    })
  }
}

function removeCopyEvents() {
  if (textRef.value) {
    const copyBtn = textRef.value.querySelectorAll('.code-block-header__copy')
    copyBtn.forEach((btn) => {
      btn.removeEventListener('click', () => {})
    })
  }
}

onMounted(() => {
  addCopyEvents()
})

onUpdated(() => {
  addCopyEvents()
})

onUnmounted(() => {
  removeCopyEvents()
})
</script>

<template>
  <div :class="wrapClass">
    <div ref="textRef" class="leading-relaxed">
      <span v-if="props.message?.content === 'loading' && !role" class="loading-box">
        <span></span>
        <span></span>
        <span></span>
      </span>
      <div v-else class="markdown-body flex flex-col" v-html="text" />
    </div>
  </div>
</template>

<style lang="scss" scoped>
.markdown-body {
  font-size: 15px;
  font-weight: 300;
}
::v-deep(.link-style) {
  color: #487bf4;
  &:hover {
    color: #275ee3;
  }
}
.loading-box{
  margin-left: 6px;
}
.loading-box span{
  display: inline-block;
  width: 5px;
  height: 5px;
  margin-right: 5px;
  border-radius: 50%;
  vertical-align: middle;
  background: rgb(182, 189, 198);
  animation: load 1.04s ease infinite;
}
.loading-box span:last-child{
  margin-right: 0px;
}
@keyframes load{
  0%{
    opacity: 1;
  }
  100%{
    opacity: 0;
  }
}
.loading-box span:nth-child(1){
  animation-delay: 0.13s;
}
.loading-box span:nth-child(2){
  animation-delay: 0.26s;
}
.loading-box span:nth-child(3){
  animation-delay: 0.39s;
}
</style>
