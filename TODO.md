# TODO

- [x] Criar uma versão inicial que atende os rascunhos dos endpoints.
- [ ] Refletir sobre a estratégia de endpoint default vs fallback e os custos do processamento.
    - Considerarei um Redis in memory compartilhado para determinar quando fazer fallback.
- [ ] Criar um job que inspeciona o health check de cada endpoint e atualiza algum status para tomada de decisão.
- [ ] Dado minha decisão de roteamento. Talvez o resty não atenda minhas necessidades. Pensar sobre.
- [ ] Remover os logs da versão final.
- [ ] Considerar usar uma lib do sonic ou json-patch para ser mais rápido no encoding e decoding do body.


## Milestones

1. Garantir que toda transação é processada.
2. Garantir que é rapido e eficiente (custo) com um bom balanço entre ambos. Eu vou tender a priorizar a menor taxa em vez do menor tempo de resposta caso precise fazer essa escolha.