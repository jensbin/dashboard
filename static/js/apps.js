const searchInput = document.getElementById('search-input');

if (!searchConfig) {
    searchConfig = []; // Fallback to prevent errors if not defined
    document.getElementById("search-input").style.display = "none"; // Hide search input
} else {
    document.getElementById("search-input").focus(); // Focus search input
}

searchInput.addEventListener('keyup', function(event) {
    if (event.key === 'Enter') {
        const searchText = this.value.trim();

        if (searchText !== '') {
            let searchUrl = searchConfig[0].URL;
            let query = searchText;

            for (const s of searchConfig) {
                if (searchText.startsWith(s.Prefix + " ")) {
                    searchUrl = s.URL;
                    query = searchText.substring(s.Prefix.length).trim();
                    break;
                }
            }
            // Default search if no prefix matches
            if (!searchUrl) {
                console.error("no url is defined.");
                searchUrl = searchConfig[0]?.URL || 'https://duckduckgo.com/?q='; // Use optional chaining
            }
            const encodedQuery = encodeURIComponent(query);

            // More robust URL handling with the URL constructor
            const url = new URL(searchUrl + encodedQuery);

            window.open(url.toString(), '_blank');
        }
    }
});
