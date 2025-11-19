<script lang="ts">
	import { goto } from '$app/navigation';
	import { user } from '$lib/stores';
	import { onMount } from 'svelte';

	onMount(() => {
		// if ($user?.role !== 'admin') {
		// 	if ($user?.permissions?.workspace?.models) {
		// 		goto('/kael/workspace/models');
		// 	} else if ($user?.permissions?.workspace?.knowledge) {
		// 		goto('/kael/workspace/knowledge');
		// 	} else if ($user?.permissions?.workspace?.prompts) {
		// 		goto('/kael/workspace/prompts');
		// 	} else if ($user?.permissions?.workspace?.tools) {
		// 		goto('/kael/workspace/tools');
		// 	} else {
		// 		goto('/kael');
		// 	}
		// } else {
		goto('/kael/');
		// }
	});
</script>
