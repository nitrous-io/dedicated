Rails.application.routes.draw do
  get '/app' => 'web_app#index'
  get '/ide' => 'web_app#ide'
  root 'pages#home'
end
