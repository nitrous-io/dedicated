Rails.application.routes.draw do
  get '/app' => 'web_app#index'
  get '/ide' => 'web_app#ide'
  get '/pricing' => 'pages#pricing'
  root 'pages#home'
end
