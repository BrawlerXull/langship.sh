// Allow side-effect CSS imports like `import "@xyflow/react/dist/style.css"`.
// Next/webpack handles the actual loading; TS just needs to know the module
// exists.
declare module "*.css";
