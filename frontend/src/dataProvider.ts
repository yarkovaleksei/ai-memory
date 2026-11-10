import type { DataProvider } from "react-admin";

// Ресурсов в фазе 0 нет: react-admin требует dataProvider, но обращаться к нему некому.
// В фазе 1 здесь появятся getList/getOne/getMany поверх привязок Wails.
const notImplemented = (method: string) => () =>
  Promise.reject(new Error(`dataProvider.${method}: ресурсы появятся в фазе 1`));

export const dataProvider: DataProvider = {
  getList: notImplemented("getList"),
  getOne: notImplemented("getOne"),
  getMany: notImplemented("getMany"),
  getManyReference: notImplemented("getManyReference"),
  create: notImplemented("create"),
  update: notImplemented("update"),
  updateMany: notImplemented("updateMany"),
  delete: notImplemented("delete"),
  deleteMany: notImplemented("deleteMany"),
};
