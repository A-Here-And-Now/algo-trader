type Price = {
    type: "price";
    symbol: string;
    time: string; // ISO string (we will parse it)
    price: number;
};

export default Price;