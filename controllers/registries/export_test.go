package registries

// export pointer to mkFreighterClient so we can modify it in test
var TestClient = &mkFreighterClient

var ActualClient = mkFreighterClient
