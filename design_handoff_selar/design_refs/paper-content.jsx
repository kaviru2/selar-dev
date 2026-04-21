// Fake PDF content — ML fundamentals corpus
const PAPERS = {
  backprop: {
    title: "Learning representations by back-propagating errors",
    authors: "Rumelhart, Hinton & Williams — Nature, 1986",
    concept: "backpropagation",
    pageNum: 4,
    abstract: null,
    sections: [
      { heading: "3. Derivation of the learning rule",
        paragraphs: [
          { text: "We begin by considering a network with feed-forward connections. The total input <sig>xⱼ</sig> to unit j is a linear function of the outputs yᵢ of the units that are connected to j and of the weights wⱼᵢ on these connections:",
            hl: null },
          { text: "A unit has a real-valued output yⱼ which is a non-linear function of its total input. In practice we use the logistic function for both its convenient derivative and its bounded range.",
            hl: null, eqn: "yⱼ = 1 / (1 + e^(−xⱼ))", eqnum: "2" },
          { text: "The aim of the procedure is to find a set of weights that ensures that for each input vector the output vector produced by the network is the same as (or sufficiently close to) the desired output vector.",
            hl: 'suggest', suggestId: 's1', badge: '3 matches' },
          { text: "If there is a fixed, finite set of input-output cases, the total error in the performance of the network with a particular set of weights can be computed by comparing the actual and desired output vectors for every case. The total error E is defined as",
            hl: null, eqn: "E = ½ Σ_c Σ_j (yⱼ,c − dⱼ,c)²", eqnum: "3" },
          { text: "To minimize E by gradient descent it is necessary to compute the partial derivative of E with respect to each weight in the network. This is simply the sum, over all input-output cases, of the partial derivatives for each case.",
            hl: 'suggest', suggestId: 's2', badge: '2 matches' },
          { text: "The forward pass, in which the units in each layer have their states determined by the input they receive from units in lower layers, is straightforward. The harder step is the backward pass, which propagates derivatives from the top layer back to the bottom one.",
            hl: 'wheat' },
        ] },
      { heading: "4. A simple example",
        paragraphs: [
          { text: "The simplest form of the learning procedure is for layered networks which have a layer of input units at the bottom; any number of intermediate layers; and a layer of output units at the top.",
            hl: null },
          { text: "Connections within a layer or from higher to lower layers are forbidden. An input vector is presented to the network by clamping the states of the input units.",
            hl: 'suggest', suggestId: 's3', badge: '1 match' },
        ] }
    ]
  },
  gradient: {
    title: "Stochastic gradient descent: an overview",
    authors: "Bottou — Proc. COMPSTAT 2010",
    concept: "gradient_descent",
  },
  linalg: {
    title: "Matrix computations, chapter 1",
    authors: "Golub & Van Loan, 4th ed.",
    concept: "linear_algebra",
  },
  attention: {
    title: "Attention is all you need",
    authors: "Vaswani et al. — NeurIPS 2017",
    concept: "transformers",
  }
};

window.PAPERS = PAPERS;
