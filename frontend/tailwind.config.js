/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{js,ts,jsx,tsx}'],
  theme: {
    extend: {
      colors: {
        'background-grey': '#A9A9A9',
        'bay-grey': '#555355',
        'header-grey': '#393939',
      },
    },
  },
  plugins: [],
}
