<script>
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import Settings from '$lib/components/admin/Settings.svelte';

	onMount(() => {
		goto('/kael/admin/settings/general');
	});
</script>

<Settings />
