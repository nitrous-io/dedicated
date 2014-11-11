Rails.application.routes.draw do
  get '/app' => 'web_app#index'
  root 'pages#home'
end
