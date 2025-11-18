<script lang="ts">
	export let name: string = '';
	export let className = 'size-5';

	import ChatGPTIcon from './ChatGPT.svelte';
	import DeepSeekIcon from './DeepSeek.svelte';
	import GrokIcon from './Grok.svelte';
	import ClaudeIcon from './Claude.svelte';
</script>

{#if (name ?? '').toLowerCase().includes('gpt')}
	<ChatGPTIcon {className} />
{:else if (name ?? '').toLowerCase().includes('deepseek')}
	<DeepSeekIcon {className} />
{:else if (name ?? '').toLowerCase().includes('grok')}
	<GrokIcon {className} />
{:else if (name ?? '').toLowerCase().includes('claude')}
	<ClaudeIcon {className} />
{:else}
	<!-- fallback left to consumer -->
{/if}
