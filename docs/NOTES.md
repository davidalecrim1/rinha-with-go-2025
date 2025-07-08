## Milestones
1. Garantir que toda transação é processada.
2. Garantir que é rapido e eficiente (custo) com um bom balanço entre ambos. Eu vou tender a priorizar a menor taxa em vez do menor tempo de resposta caso precise fazer essa escolha.

## Algoritmo de Retry
- Usa channels e goroutines para processar.
- Goroutines fazem retry com exponential backoff
  - Elas decidem se mudam para o fallback.
    - Caso não consiga processar na API default em 200 milisegundos. Muda para o fallback.
    - Caso não consiga processar em 200 milisegundos. Volta para a fila.
- Única goroutine para observar o health check das APIs e salvar em um redis com status dos serviços caso precisar usar essa informação.

O foco agora é garantir que todas as requisições sejam processadas. O que pela leitura do script da K6 vi que não seria possível de forma sincrona mantendo a requisição HTTP aberta na minha API.